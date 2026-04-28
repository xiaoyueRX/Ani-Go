package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	if err := database.Init(":memory:"); err != nil {
		t.Fatalf("初始化测试数据库失败: %v", err)
	}
	if err := database.DB.AutoMigrate(
		&database.Subscription{},
		&database.Episode{},
		&database.DownloadRecord{},
		&database.Setting{},
	); err != nil {
		t.Fatalf("自动迁移失败: %v", err)
	}
}

func seedSubscription(t *testing.T, sub database.Subscription) database.Subscription {
	t.Helper()
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatalf("创建测试订阅失败: %v", err)
	}
	return sub
}

func newTestServer() *Server {
	return &Server{
		downloader:        nil,
		triggerSupplement: nil,
	}
}

func TestHandleListSubscriptions_Empty(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	w := httptest.NewRecorder()
	s.handleListSubscriptions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态 200, 实际 %d", w.Code)
	}

	var subscriptions []subscriptionResponse
	json.NewDecoder(w.Body).Decode(&subscriptions)
	if len(subscriptions) != 0 {
		t.Errorf("期望 0 个订阅, 实际 %d", len(subscriptions))
	}
}

func TestHandleListSubscriptions_WithData(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	seedSubscription(t, database.Subscription{
		TitleCN: "鬼灭之刃", BangumiID: "12345", Enabled: true,
	})
	sub2 := seedSubscription(t, database.Subscription{
		TitleCN: "迷宫饭", BangumiID: "67890", Enabled: true,
	})
	database.DB.Model(&sub2).Update("enabled", false)

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	w := httptest.NewRecorder()
	s.handleListSubscriptions(w, req)

	var subs []subscriptionResponse
	json.NewDecoder(w.Body).Decode(&subs)
	if len(subs) != 2 {
		t.Errorf("期望 2 个订阅, 实际 %d", len(subs))
	}
}

func TestHandleListSubscriptions_FilterEnabled(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	seedSubscription(t, database.Subscription{
		TitleCN: "鬼灭之刃", BangumiID: "12345", Enabled: true,
	})
	sub2 := seedSubscription(t, database.Subscription{
		TitleCN: "迷宫饭", BangumiID: "67890", Enabled: true,
	})
	// GORM default:true 导致 false 被覆盖，用 Update 强制设 false
	database.DB.Model(&sub2).Update("enabled", false)

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions?enabled=true", nil)
	w := httptest.NewRecorder()
	s.handleListSubscriptions(w, req)

	var subs []subscriptionResponse
	json.NewDecoder(w.Body).Decode(&subs)
	if len(subs) != 1 {
		t.Errorf("期望 1 个启用的订阅, 实际 %d", len(subs))
	}
	if subs[0].TitleCN != "鬼灭之刃" {
		t.Errorf("期望 鬼灭之刃, 实际 %s", subs[0].TitleCN)
	}
}

func TestHandleCreateSubscription_Success(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	body := bytes.NewBufferString(`{"title_cn":"鬼灭之刃 游郭篇","bangumi_id":"12345","subgroup_name":"千夏字幕组"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions", body)
	w := httptest.NewRecorder()
	s.handleCreateSubscription(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("期望状态 201, 实际 %d: %s", w.Code, w.Body.String())
	}

	var resp subscriptionResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.TitleCN != "鬼灭之刃 游郭篇" {
		t.Errorf("期望 鬼灭之刃 游郭篇, 实际 %s", resp.TitleCN)
	}
	if !resp.Enabled {
		t.Error("新建订阅默认应启用")
	}
}

func TestHandleCreateSubscription_MissingTitle(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	body := bytes.NewBufferString(`{"bangumi_id":"12345"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions", body)
	w := httptest.NewRecorder()
	s.handleCreateSubscription(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("期望状态 400, 实际 %d", w.Code)
	}
}

func TestHandleGetSubscription_NotFound(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions/999", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()
	s.handleGetSubscription(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("期望状态 404, 实际 %d", w.Code)
	}
}

func TestHandleGetSubscription_WithEpisodes(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	sub := seedSubscription(t, database.Subscription{
		TitleCN: "鬼灭之刃", BangumiID: "12345",
	})
	database.DB.Create(&database.Episode{
		SubscriptionID: sub.ID,
		Season:         1,
		Number:         1,
		Title:          "第1话",
		Status:         "downloaded",
		TorrentHash:    "abc123",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleGetSubscription(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望状态 200, 实际 %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	eps, ok := resp["episodes"].([]interface{})
	if !ok || len(eps) != 1 {
		t.Errorf("期望 1 个剧集, 实际 %v", resp["episodes"])
	}
}

func TestHandleUpdateSubscription_Partial(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	seedSubscription(t, database.Subscription{
		TitleCN: "鬼灭之刃", BangumiID: "12345", SubgroupName: "千夏",
	})

	body := bytes.NewBufferString(`{"subgroup_name":"喵萌奶茶屋","total_episodes":12}`)
	req := httptest.NewRequest(http.MethodPut, "/api/subscriptions/1", body)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleUpdateSubscription(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望状态 200, 实际 %d: %s", w.Code, w.Body.String())
	}

	var resp subscriptionResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.SubgroupName != "喵萌奶茶屋" {
		t.Errorf("SubgroupName 期望 喵萌奶茶屋, 实际 %s", resp.SubgroupName)
	}
	if resp.TotalEpisodes != 12 {
		t.Errorf("TotalEpisodes 期望 12, 实际 %d", resp.TotalEpisodes)
	}
	if resp.TitleCN != "鬼灭之刃" {
		t.Errorf("TitleCN 应保持不变, 实际 %s", resp.TitleCN)
	}
}

func TestHandleDeleteSubscription_Success(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	sub := seedSubscription(t, database.Subscription{
		TitleCN: "鬼灭之刃", BangumiID: "12345",
	})
	database.DB.Create(&database.Episode{
		SubscriptionID: sub.ID, Season: 1, Number: 1, Title: "第1话",
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/subscriptions/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleDeleteSubscription(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态 200, 实际 %d", w.Code)
	}

	// 验证订阅已删除
	var count int64
	database.DB.Model(&database.Subscription{}).Count(&count)
	if count != 0 {
		t.Error("订阅应被删除")
	}

	// 验证关联剧集已级联删除
	database.DB.Model(&database.Episode{}).Count(&count)
	if count != 0 {
		t.Errorf("关联剧集应级联删除, 实际还有 %d 条", count)
	}
}

func TestHandleGetSettings_Empty(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	w := httptest.NewRecorder()
	s.handleGetSettings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态 200, 实际 %d", w.Code)
	}
}

func TestHandleUpdateSettings_AddAndUpdate(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	body := bytes.NewBufferString(`{"settings":{"theme":"dark","language":"zh"}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/settings", body)
	w := httptest.NewRecorder()
	s.handleUpdateSettings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态 200, 实际 %d", w.Code)
	}

	var settings []database.Setting
	database.DB.Find(&settings)
	if len(settings) != 2 {
		t.Errorf("期望 2 个设置项, 实际 %d", len(settings))
	}
}

func TestHandleListDownloads_NoDownloader(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/downloads", nil)
	w := httptest.NewRecorder()
	s.handleListDownloads(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("期望状态 503, 实际 %d", w.Code)
	}
}

func TestHandleTriggerSupplement_InvalidID(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions/abc/trigger-supplement", nil)
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	s.handleTriggerSupplement(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("期望状态 400, 实际 %d", w.Code)
	}
}

func TestHandleTriggerSupplement_NotFound(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions/999/trigger-supplement", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()
	s.handleTriggerSupplement(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("期望状态 404, 实际 %d", w.Code)
	}
}

func TestHandleTriggerSupplement_Disabled(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()

	sub := seedSubscription(t, database.Subscription{
		TitleCN: "鬼灭之刃", BangumiID: "12345", Enabled: true,
	})
	database.DB.Model(&sub).Update("enabled", false)

	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions/1/trigger-supplement", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleTriggerSupplement(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("期望状态 400, 实际 %d: %s", w.Code, w.Body.String())
	}
}

// TestToSubscriptionResponse 验证响应格式
func TestToSubscriptionResponse(t *testing.T) {
	sub := database.Subscription{
		TitleCN: "测试番剧", Year: 2024, Season: 2, TotalEpisodes: 26, Enabled: true,
	}
	resp := toSubscriptionResponse(sub)
	if resp.TitleCN != "测试番剧" {
		t.Errorf("TitleCN = %s, 期望 测试番剧", resp.TitleCN)
	}
	if resp.Year != 2024 {
		t.Errorf("Year = %d, 期望 2024", resp.Year)
	}
	if resp.Season != 2 {
		t.Errorf("Season = %d, 期望 2", resp.Season)
	}
}

func TestToEpisodeResponse(t *testing.T) {
	ep := database.Episode{
		Season: 1, Number: 2.5, Title: "特别篇", Status: "downloaded", TorrentHash: "abc",
	}
	resp := toEpisodeResponse(ep)
	if resp.Number != 2.5 {
		t.Errorf("Number = %f, 期望 2.5", resp.Number)
	}
	if resp.Status != "downloaded" {
		t.Errorf("Status = %s, 期望 downloaded", resp.Status)
	}
}

// stubDownloader 用于测试下载列表的桩下载器
type stubDownloader struct {
	tasks []core.DownloadTask
}

func (s *stubDownloader) Name() string { return "stub" }
func (s *stubDownloader) Add(ctx context.Context, item core.TorrentItem, path string) error {
	return nil
}
func (s *stubDownloader) List(ctx context.Context) ([]core.DownloadTask, error) {
	return s.tasks, nil
}
func (s *stubDownloader) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	return core.DownloadTask{}, nil
}
func (s *stubDownloader) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	return nil
}
func (s *stubDownloader) IsAvailable(ctx context.Context) bool { return true }

func TestHandleListDownloads_WithData(t *testing.T) {
	s := &Server{
		downloader: &stubDownloader{
			tasks: []core.DownloadTask{
				{Hash: "abc", Name: "test.torrent", Status: "downloading", Progress: 0.5, Size: 1024, Done: 512},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/downloads", nil)
	w := httptest.NewRecorder()
	s.handleListDownloads(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态 200, 实际 %d", w.Code)
	}
}
