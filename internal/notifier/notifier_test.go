package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&database.Setting{}); err != nil {
		t.Fatalf("自动迁移表失败: %v", err)
	}
	return db
}

// ------------------------------------------------------------
// Signal 测试
// ------------------------------------------------------------

func TestSignalNotifier_SendSuccess(t *testing.T) {
	var receivedBody map[string]any
	var receivedAuthHeader string
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/send" {
			http.NotFound(w, r)
			return
		}
		receivedContentType = r.Header.Get("Content-Type")
		receivedAuthHeader = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	notifier := NewSignalNotifierWithParams(server.URL, "+8613800000000", []string{"+8613900000001", " +8613900000002 ", "+8613900000001"})
	notifier.httpClient = server.Client()

	if notifier.Name() != "Signal" {
		t.Fatalf("期望名称为 Signal，实际为: %s", notifier.Name())
	}

	err := notifier.Send(context.Background(), "新番更新", "第 1 集已下载完成")
	if err != nil {
		t.Fatalf("发送 Signal 失败: %v", err)
	}

	if !strings.Contains(receivedContentType, "application/json") {
		t.Errorf("Content-Type 错误: %s", receivedContentType)
	}
	if receivedBody["number"] != "+8613800000000" {
		t.Errorf("发送者号码不匹配: %v", receivedBody["number"])
	}
	expectedMsg := "【新番更新】\n第 1 集已下载完成"
	if receivedBody["message"] != expectedMsg {
		t.Errorf("消息正文不匹配，期望 %q，实际 %q", expectedMsg, receivedBody["message"])
	}

	recipients, ok := receivedBody["recipients"].([]any)
	if !ok || len(recipients) != 2 {
		t.Fatalf("接收者列表去重后应为 2 个，实际为: %v", receivedBody["recipients"])
	}
	_ = receivedAuthHeader
}

func TestSignalNotifier_HTTPStatusErrorSnippet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid recipient phone number"}`))
	}))
	defer server.Close()

	notifier := NewSignalNotifierWithParams(server.URL, "+8613800000000", []string{"+8613900000001"})
	notifier.httpClient = server.Client()

	err := notifier.Send(context.Background(), "测试", "内容")
	if err == nil {
		t.Fatalf("期望返回错误，实际为 nil")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "Invalid recipient") {
		t.Fatalf("错误信息未包含状态码或响应内容: %v", err)
	}
}

func TestSignalNotifier_Validation(t *testing.T) {
	// 缺失配置
	n := NewSignalNotifierWithParams("", "", nil)
	if err := n.Send(context.Background(), "t", "m"); err == nil {
		t.Fatalf("期望空配置报错")
	}

	// 非法 URL
	n2 := NewSignalNotifierWithParams("ftp://invalid-url", "+8613800000000", []string{"+8613900000001"})
	if err := n2.Send(context.Background(), "t", "m"); err == nil {
		t.Fatalf("期望非法 URL 报错")
	}
}

// ------------------------------------------------------------
// WeChat 测试
// ------------------------------------------------------------

func TestWeChatNotifier_CustomMessage(t *testing.T) {
	var tokenCalls int32
	var sendCalls int32
	var receivedSendBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			atomic.AddInt32(&tokenCalls, 1)
			if r.URL.Query().Get("appid") != "wx_app_123" || r.URL.Query().Get("secret") != "wx_secret_456" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"errcode":40013,"errmsg":"invalid appid"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"access_token":"mock_token_abc","expires_in":7200,"errcode":0,"errmsg":"ok"}`))
		case "/cgi-bin/message/custom/send":
			atomic.AddInt32(&sendCalls, 1)
			token := r.URL.Query().Get("access_token")
			if token != "mock_token_abc" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"errcode":40001,"errmsg":"invalid credential"}`))
				return
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedSendBody)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	notifier := NewWeChatNotifierWithParams("wx_app_123", "wx_secret_456", "user_open_id", "")
	notifier.BaseURL = server.URL
	notifier.httpClient = server.Client()

	if notifier.Name() != "WeChat" {
		t.Fatalf("期望名称 WeChat，实际: %s", notifier.Name())
	}

	err := notifier.Send(context.Background(), "下载通知", "番剧已入库")
	if err != nil {
		t.Fatalf("微信客服消息发送失败: %v", err)
	}

	if atomic.LoadInt32(&tokenCalls) != 1 || atomic.LoadInt32(&sendCalls) != 1 {
		t.Fatalf("调用次数不符合预期: tokenCalls=%d, sendCalls=%d", tokenCalls, sendCalls)
	}

	if receivedSendBody["touser"] != "user_open_id" {
		t.Errorf("touser 不匹配: %v", receivedSendBody["touser"])
	}
	textMap, _ := receivedSendBody["text"].(map[string]any)
	if textMap["content"] != "【下载通知】\n番剧已入库" {
		t.Errorf("文本内容不匹配: %v", textMap["content"])
	}

	// 第二次发送复用 token
	err = notifier.Send(context.Background(), "第二条", "复用token")
	if err != nil {
		t.Fatalf("第二条消息发送失败: %v", err)
	}
	if atomic.LoadInt32(&tokenCalls) != 1 {
		t.Fatalf("应当复用未过期的 token，tokenCalls=%d", tokenCalls)
	}
}

func TestWeChatNotifier_TemplateMessage(t *testing.T) {
	var receivedSendBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"access_token":"tpl_token","expires_in":7200,"errcode":0}`))
		case "/cgi-bin/message/template/send":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedSendBody)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	notifier := NewWeChatNotifierWithParams("wx_app", "wx_secret", "user_openid", "template_xyz")
	notifier.BaseURL = server.URL
	notifier.httpClient = server.Client()

	err := notifier.Send(context.Background(), "标题", "内容正文")
	if err != nil {
		t.Fatalf("发送模板消息失败: %v", err)
	}

	if receivedSendBody["template_id"] != "template_xyz" {
		t.Fatalf("模板ID未正确传递: %v", receivedSendBody["template_id"])
	}
	data, ok := receivedSendBody["data"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 data 结构体")
	}
	firstMap, _ := data["first"].(map[string]any)
	if firstMap["value"] != "标题" {
		t.Errorf("first value 不匹配: %v", firstMap["value"])
	}
}

func TestWeChatNotifier_TokenRefreshAndRetry(t *testing.T) {
	var tokenCount int32
	var sendCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			c := atomic.AddInt32(&tokenCount, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"access_token":"token_v%d","expires_in":7200,"errcode":0}`, c)))
		case "/cgi-bin/message/custom/send":
			atomic.AddInt32(&sendCount, 1)
			token := r.URL.Query().Get("access_token")
			if token == "token_v1" {
				// 模拟旧 token 失效
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"errcode":42001,"errmsg":"token expired"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	notifier := NewWeChatNotifierWithParams("wx_app", "wx_secret", "user_id", "")
	notifier.BaseURL = server.URL
	notifier.httpClient = server.Client()

	err := notifier.Send(context.Background(), "测试标题", "重试测试")
	if err != nil {
		t.Fatalf("token 自动重试发送失败: %v", err)
	}

	if atomic.LoadInt32(&tokenCount) != 2 || atomic.LoadInt32(&sendCount) != 2 {
		t.Fatalf("应触发一次重试刷新: tokenCount=%d, sendCount=%d", tokenCount, sendCount)
	}
}

func TestWeChatNotifier_ComputeTokenExpiry(t *testing.T) {
	// expiresIn > 60: 留 60s 缓冲
	e1 := computeTokenExpiry(7200)
	if time.Until(e1) < 7100*time.Second || time.Until(e1) > 7145*time.Second {
		t.Errorf("e1 超出预期区间: %v", time.Until(e1))
	}

	// expiresIn <= 60: 取一半缓冲
	e2 := computeTokenExpiry(30)
	if time.Until(e2) < 10*time.Second || time.Until(e2) > 16*time.Second {
		t.Errorf("e2 超出预期区间: %v", time.Until(e2))
	}

	// <= 0: 即时过期
	e3 := computeTokenExpiry(0)
	if time.Until(e3) > 100*time.Millisecond {
		t.Errorf("e3 应当接近立即过期: %v", time.Until(e3))
	}
}

// ------------------------------------------------------------
// Builder 与 环境变量 > 数据库 优先律测试
// ------------------------------------------------------------

func TestBuildAllNotifiersFromSettingsAndEnv_Priority(t *testing.T) {
	// 保存原环境变量并清理
	origEnv := os.Getenv("TELEGRAM_BOT_TOKEN")
	origChat := os.Getenv("TELEGRAM_CHAT_ID")
	origSigURL := os.Getenv("SIGNAL_API_URL")
	origSigSender := os.Getenv("SIGNAL_SENDER")
	origSigRecipients := os.Getenv("SIGNAL_RECIPIENTS")
	origWechatApp := os.Getenv("WECHAT_APP_ID")
	origWechatSecret := os.Getenv("WECHAT_APP_SECRET")
	origWechatUser := os.Getenv("WECHAT_USER_ID")
	defer func() {
		os.Setenv("TELEGRAM_BOT_TOKEN", origEnv)
		os.Setenv("TELEGRAM_CHAT_ID", origChat)
		os.Setenv("SIGNAL_API_URL", origSigURL)
		os.Setenv("SIGNAL_SENDER", origSigSender)
		os.Setenv("SIGNAL_RECIPIENTS", origSigRecipients)
		os.Setenv("WECHAT_APP_ID", origWechatApp)
		os.Setenv("WECHAT_APP_SECRET", origWechatSecret)
		os.Setenv("WECHAT_USER_ID", origWechatUser)
	}()

	db := newTestDB(t)
	oldDB := database.DB
	database.DB = db
	defer func() { database.DB = oldDB }()

	// 1. 仅数据库有配置
	os.Unsetenv("SIGNAL_API_URL")
	os.Unsetenv("SIGNAL_SENDER")
	os.Unsetenv("SIGNAL_RECIPIENTS")
	db.Save(&database.Setting{Key: "SIGNAL_API_URL", Value: "http://db-signal:8080"})
	db.Save(&database.Setting{Key: "SIGNAL_SENDER", Value: "+8613800000000"})
	db.Save(&database.Setting{Key: "SIGNAL_RECIPIENTS", Value: "+8613900000000"})

	notifiers := BuildAllNotifiersFromSettingsAndEnv()
	var signalNotifier *SignalNotifier
	for _, n := range notifiers {
		if sn, ok := n.(*SignalNotifier); ok {
			signalNotifier = sn
			break
		}
	}
	if signalNotifier == nil {
		t.Fatalf("应从数据库成功加载 Signal 通知器")
	}
	if signalNotifier.APIURL != "http://db-signal:8080" {
		t.Errorf("API URL 应为 DB 中的配置: %s", signalNotifier.APIURL)
	}

	// 2. 环境变量优先律覆写
	os.Setenv("SIGNAL_API_URL", "http://env-signal:9090")
	notifiers2 := BuildAllNotifiersFromSettingsAndEnv()
	var signalNotifier2 *SignalNotifier
	for _, n := range notifiers2 {
		if sn, ok := n.(*SignalNotifier); ok {
			signalNotifier2 = sn
			break
		}
	}
	if signalNotifier2 == nil || signalNotifier2.APIURL != "http://env-signal:9090" {
		t.Fatalf("环境变量未成功优先覆盖数据库配置: %+v", signalNotifier2)
	}

	// 3. 验证微信公众号注册进 BuildAllNotifiers
	os.Setenv("WECHAT_APP_ID", "wx_env_id")
	os.Setenv("WECHAT_APP_SECRET", "wx_env_sec")
	os.Setenv("WECHAT_USER_ID", "wx_env_usr")
	notifiers3 := BuildAllNotifiersFromSettingsAndEnv()
	var wechatFound bool
	for _, n := range notifiers3 {
		if wn, ok := n.(*WeChatNotifier); ok {
			wechatFound = true
			if wn.AppID != "wx_env_id" {
				t.Errorf("WeChat AppID 不匹配: %s", wn.AppID)
			}
		}
	}
	if !wechatFound {
		t.Fatalf("WeChatNotifier 未在 BuildAllNotifiers 中注册")
	}
}

// ------------------------------------------------------------
// TemplateManager 顶层包兼容性测试
// ------------------------------------------------------------

func TestTopLevelTemplateManager_Compatibility(t *testing.T) {
	tm := NewTemplateManager()
	if err := tm.Add("custom.event", "自定义模板：{{.Title}}"); err != nil {
		t.Fatalf("Add 模板失败: %v", err)
	}

	res, err := tm.Render("custom.event", map[string]any{"Title": "测试"})
	if err != nil || !strings.Contains(res, "自定义模板：测试") {
		t.Fatalf("Render 失败: res=%q, err=%v", res, err)
	}

	if err := ValidateTemplate("{{.Title | upper}}"); err != nil {
		t.Fatalf("ValidateTemplate 合法模板报错: %v", err)
	}
	if err := ValidateTemplate("{{.Title | unclosed"); err == nil {
		t.Fatalf("ValidateTemplate 语法错误未报错")
	}
}
