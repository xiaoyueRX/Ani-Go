package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

type oauthTransport func(*http.Request) (*http.Response, error)

func (f oauthTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBangumiAuthenticatedLinkAndCallback(t *testing.T) {
	setupTestDB(t)
	if err := auth.InitSecret(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BANGUMI_CLIENT_ID", "test-client")
	t.Setenv("BANGUMI_CLIENT_SECRET", "test-secret")
	user := database.User{Username: "owner", PasswordHash: "unused", TokenVersion: 2}
	if err := database.DB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateToken(user.Username, user.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	handler := auth.AuthMiddleware(mux)
	link := httptest.NewRequest(http.MethodGet, "https://example.test/api/bangumi/auth/link", nil)
	link.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, link)
	var payload struct{ URL string }
	if err := json.Unmarshal(w.Body.Bytes(), &payload); w.Code != 200 || err != nil {
		t.Fatalf("获取授权链接失败 %d %v", w.Code, err)
	}
	href, err := url.Parse(payload.URL)
	if err != nil || href.Query().Get("state") == "" || len(w.Result().Cookies()) != 1 {
		t.Fatal("授权链接缺少会话标识")
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("回调凭证属性不完整")
	}
	original := httpx.Default
	t.Cleanup(func() { httpx.Default = original })
	calls := 0
	httpx.Default = &http.Client{Transport: oauthTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"access_token":"test-access"}`
		if r.URL.Path == "/v0/me" {
			body = `{"username":"bound-user"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	callback := href.Query().Get("redirect_uri") + "?code=test-code&state=" + url.QueryEscape(href.Query().Get("state"))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, callback, nil))
	if w.Code != http.StatusBadRequest || calls != 0 {
		t.Fatalf("缺失浏览器会话时应在换证前结束")
	}
	for i, want := range []int{http.StatusFound, http.StatusBadRequest} {
		req := httptest.NewRequest(http.MethodGet, callback, nil)
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("第 %d 次回调得到 %d，期望 %d", i+1, w.Code, want)
		}
	}
	if calls != 2 || getSettingValue("BGMTV_USER_TOKEN") != "test-access" || getSettingValue("BGMTV_USERNAME") != "bound-user" {
		t.Fatal("授权绑定未按预期保存")
	}
}

func TestImageProxyRasterResponse(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		body []byte
		code int
	}{
		{encoded.Bytes(), http.StatusOK},
		{[]byte("upstream maintenance page"), http.StatusUnsupportedMediaType},
	} {
		w := httptest.NewRecorder()
		serveRasterImage(w, &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(test.body))})
		if w.Code != test.code {
			t.Fatalf("预期 %d，得到 %d", test.code, w.Code)
		}
		if w.Code == 200 && (w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-Content-Type-Options") != "nosniff" || !bytes.Equal(w.Body.Bytes(), test.body)) {
			t.Fatal("代理图片响应无效")
		}
	}
}

func TestSettingsPreserveRotatedMCPTokenAndDownloader(t *testing.T) {
	setupTestDB(t)
	initial := &stubDownloader{}
	dyn := downloader.NewDynamicDownloader(initial)
	s := &Server{downloader: dyn}
	t.Setenv("DEFAULT_DOWNLOADER", "qbittorrent")
	t.Setenv("QB_HOST", "http://configured.example")
	t.Setenv("QB_USER", "")
	t.Setenv("QB_USERNAME", "legacy-user")
	if cfg := effectiveDownloaders(map[string]string{"QB_HOST": "http://old.example"}); cfg.QBittorrent.Host != "http://configured.example" || cfg.QBittorrent.Username != "legacy-user" {
		t.Fatal("热重载配置应遵循启动优先级并支持旧配置键")
	}
	rotated := httptest.NewRecorder()
	s.handleGenerateMCPToken(rotated, httptest.NewRequest(http.MethodPost, "/api/mcp/token/generate", nil))
	var result map[string]string
	if err := json.Unmarshal(rotated.Body.Bytes(), &result); err != nil || result["token"] == "" {
		t.Fatal("令牌生成失败")
	}
	settings := httptest.NewRecorder()
	s.handleGetSettings(settings, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if strings.Contains(settings.Body.String(), result["token"]) || !strings.Contains(settings.Body.String(), "MCP_TOKEN_CONFIGURED") {
		t.Fatal("设置应仅返回令牌配置状态")
	}
	write := httptest.NewRecorder()
	s.handleUpdateSettings(write, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"settings":{"MCP_TOKEN":"obsolete","QB_HOST":"http://old.example","language":"de"}}`)))
	if write.Code != 200 || getSettingValue("MCP_TOKEN") != result["token"] || dyn.GetCurrent() != initial {
		t.Fatal("无关更新修改了凭据或运行中的下载器")
	}
}

func TestBatchDownloadNativeStates(t *testing.T) {
	s := &Server{downloader: &stubDownloader{tasks: []core.DownloadTask{
		{Status: "active"}, {Status: "stalledUP"}, {Status: "queuedDL"}, {Status: "stopped"},
		{Status: "pausedDL"}, {Status: "stoppedUP"}, {Status: "error"},
	}}}
	for _, handle := range []http.HandlerFunc{s.handlePauseAllDownloads, s.handleResumeAllDownloads} {
		w := httptest.NewRecorder()
		handle(w, httptest.NewRequest(http.MethodPost, "/", nil))
		var result struct{ Count int }
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Count != 3 {
			t.Fatalf("批量动作的任务数无效 %s", w.Body)
		}
	}
}

func TestListingSubscriptionsDoesNotTriggerDownloads(t *testing.T) {
	setupTestDB(t)
	sub := seedSubscription(t, database.Subscription{TitleCN: "暂停订阅"})
	if err := database.DB.Model(&sub).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	trigger := make(chan struct{}, 1)
	s := &Server{triggerSupplement: func(context.Context, uint) error { trigger <- struct{}{}; return nil }}
	w := httptest.NewRecorder()
	s.handleListSubscriptions(w, httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil))
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	select {
	case <-trigger:
		t.Fatal("只读查询启动了下载")
	case <-time.After(30 * time.Millisecond):
	}
}

func TestPollingIntervalWritesAreValidatedBeforeSave(t *testing.T) {
	setupTestDB(t)
	s := &Server{}
	put := func(values map[string]string) *httptest.ResponseRecorder {
		body, err := json.Marshal(settingsRequest{Settings: values})
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		s.handleUpdateSettings(w, httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body)))
		return w
	}
	for key, value := range map[string]string{
		"RSS_INTERVAL": "0s", "ORGANIZER_INTERVAL": "-5m", "SUPPLEMENT_INTERVAL": "9223372036854775808ns",
		"BGMTV_SYNC_INTERVAL": "x", "SEED_CLEANUP_INTERVAL": "0",
	} {
		w := put(map[string]string{key: value, "theme": "must-not-persist"})
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), key) || getSettingValue("theme") != "" {
			t.Fatalf("轮询设置 %s 未被整批拒绝 %d %s", key, w.Code, w.Body.String())
		}
	}
	w := put(map[string]string{"RSS_INTERVAL": " 2h30m ", "SEED_CLEANUP_MIN_SEED_TIME": "0s", "SEED_CLEANUP_MIN_RATIO": "0"})
	if w.Code != 200 || getSettingValue("RSS_INTERVAL") != "2h30m" || getSettingValue("SEED_CLEANUP_MIN_SEED_TIME") != "0s" {
		t.Fatalf("有效间隔或零阈值保存失败 %s", w.Body.String())
	}
	w = put(map[string]string{"RSS_INTERVAL": " "})
	if w.Code != 200 || getSettingValue("RSS_INTERVAL") != "" {
		t.Fatalf("清除轮询间隔失败")
	}
}
