package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
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
		TitleCN: "鬼灭之刃", BangumiID: "12345", Enabled: &[]bool{true}[0],
	})
	sub2 := seedSubscription(t, database.Subscription{
		TitleCN: "迷宫饭", BangumiID: "67890", Enabled: &[]bool{true}[0],
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
		TitleCN: "鬼灭之刃", BangumiID: "12345", Enabled: &[]bool{true}[0],
	})
	sub2 := seedSubscription(t, database.Subscription{
		TitleCN: "迷宫饭", BangumiID: "67890", Enabled: &[]bool{true}[0],
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

	// 测试回收站列表接口
	recycleReq := httptest.NewRequest(http.MethodGet, "/api/subscriptions?deleted=true", nil)
	recycleW := httptest.NewRecorder()
	s.handleListSubscriptions(recycleW, recycleReq)
	if recycleW.Code != http.StatusOK {
		t.Errorf("回收站查询失败: 状态码 %d", recycleW.Code)
	}
	var deletedSubs []subscriptionResponse
	json.Unmarshal(recycleW.Body.Bytes(), &deletedSubs)
	if len(deletedSubs) != 1 {
		t.Errorf("期望回收站有 1 条记录, 实际有 %d 条", len(deletedSubs))
	}
	if deletedSubs[0].DeletedAt == nil {
		t.Error("回收站返回数据的 DeletedAt 应不为 nil")
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
		TitleCN: "鬼灭之刃", BangumiID: "12345", Enabled: &[]bool{true}[0],
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
		TitleCN: "测试番剧", Year: 2024, Season: 2, TotalEpisodes: 26, Enabled: &[]bool{true}[0],
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
func (s *stubDownloader) Pause(ctx context.Context, hash string) error {
	return nil
}
func (s *stubDownloader) Resume(ctx context.Context, hash string) error {
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

func TestHandleDownloadActions(t *testing.T) {
	s := &Server{
		downloader: &stubDownloader{},
	}

	// 1. Create download
	body := strings.NewReader(`{"title":"Test Anime","magnet":"magnet:?xt=urn:btih:test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/downloads", body)
	w := httptest.NewRecorder()
	s.handleCreateDownload(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("handleCreateDownload 期望 200, 实际 %d: %s", w.Code, w.Body.String())
	}

	// 2. Pause download
	req = httptest.NewRequest(http.MethodPost, "/api/downloads/abc/pause", nil)
	req.SetPathValue("hash", "abc")
	w = httptest.NewRecorder()
	s.handlePauseDownload(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("handlePauseDownload 期望 200, 实际 %d: %s", w.Code, w.Body.String())
	}

	// 3. Resume download
	req = httptest.NewRequest(http.MethodPost, "/api/downloads/abc/resume", nil)
	req.SetPathValue("hash", "abc")
	w = httptest.NewRecorder()
	s.handleResumeDownload(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("handleResumeDownload 期望 200, 实际 %d: %s", w.Code, w.Body.String())
	}

	// 4. Delete download
	req = httptest.NewRequest(http.MethodDelete, "/api/downloads/abc?delete_files=true", nil)
	req.SetPathValue("hash", "abc")
	w = httptest.NewRecorder()
	s.handleDeleteDownload(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("handleDeleteDownload 期望 200, 实际 %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBatchDownloadActions(t *testing.T) {
	s := &Server{
		downloader: &stubDownloader{
			tasks: []core.DownloadTask{
				{Hash: "task1", Status: "downloading"},
				{Hash: "task2", Status: "paused"},
			},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/downloads/pause-all", nil)
	w := httptest.NewRecorder()
	s.handlePauseAllDownloads(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("handlePauseAllDownloads 期望 200, 实际 %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/downloads/resume-all", nil)
	w = httptest.NewRecorder()
	s.handleResumeAllDownloads(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("handleResumeAllDownloads 期望 200, 实际 %d", w.Code)
	}
}

func TestHandleNotificationLogsAndStats(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/notifications/logs?page=1&pageSize=10", nil)
	w := httptest.NewRecorder()
	s.handleListNotificationLogs(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("handleListNotificationLogs 期望 200, 实际 %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/notifications/stats", nil)
	w = httptest.NewRecorder()
	s.handleGetNotificationStats(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("handleGetNotificationStats 期望 200, 实际 %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/notifications/logs?days=30", nil)
	w = httptest.NewRecorder()
	s.handleClearNotificationLogs(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("handleClearNotificationLogs 期望 200, 实际 %d", w.Code)
	}
}

func TestHandleGenerateMCPToken(t *testing.T) {
	setupTestDB(t)
	s := &Server{}

	req := httptest.NewRequest(http.MethodPost, "/api/mcp/token/generate", nil)
	w := httptest.NewRecorder()
	s.handleGenerateMCPToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleGenerateMCPToken 期望 200, 实际 %d", w.Code)
	}

	var res map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	token, ok := res["token"]
	if !ok || len(token) != 64 {
		t.Errorf("期望 64 位十六进制 token, 实际: %s", token)
	}

	// 验证持久化
	var setting database.Setting
	if err := database.DB.Where("key = ?", "MCP_TOKEN").First(&setting).Error; err != nil {
		t.Fatalf("数据库中未查找到持久化的 MCP_TOKEN: %v", err)
	}
	if setting.Value != token {
		t.Errorf("持久化值不一致: 期望 %s, 实际 %s", token, setting.Value)
	}
}

func TestHandlePreviewOrganize(t *testing.T) {
	setupTestDB(t)

	// 1. 测试未提供 organizer 时返回 503
	sNoOrg := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/organize/preview", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	sNoOrg.handlePreviewOrganize(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("未配置 organizer 时期望 503, 实际 %d", w.Code)
	}

	// 2. 测试配置 organizer 后的单文件预览
	tmpDir := t.TempDir()
	org := organizer.New(
		"{title_cn}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
		"", "",
		tmpDir, tmpDir,
		true,
		nil,
	)
	s := &Server{organizer: org}

	body := organizePreviewRequest{
		FilePath: "/downloads/dungeon_01.mkv",
		Anime: core.Anime{
			TitleCN: "迷宫饭",
			TitleEN: "Delicious in Dungeon",
			Type:    "TV",
		},
		Episode: core.Episode{
			Season: 1,
			Number: 1,
		},
	}
	raw, _ := json.Marshal(body)
	req = httptest.NewRequest(http.MethodPost, "/api/organize/preview", bytes.NewReader(raw))
	w = httptest.NewRecorder()
	s.handlePreviewOrganize(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("单项预览期望 200, 实际 %d, 响应: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int                     `json:"code"`
		Data organizer.PreviewResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析预览响应失败: %v", err)
	}
	if resp.Code != 0 {
		t.Errorf("期望 code 0, 实际 %d", resp.Code)
	}
	if resp.Data.Action != "hardlink" {
		t.Errorf("期望 action hardlink, 实际 %s", resp.Data.Action)
	}
	if !strings.Contains(resp.Data.FinalPath, "Delicious in Dungeon S01E01.mkv") {
		t.Errorf("FinalPath 不匹配: %s", resp.Data.FinalPath)
	}

	// 3. 测试批量预览
	batchBody := organizePreviewRequest{
		Items: []organizer.PreviewItem{
			{
				FilePath: "/downloads/dungeon_01.mkv",
				Anime:    core.Anime{TitleCN: "迷宫饭", TitleEN: "Delicious in Dungeon", Type: "TV"},
				Episode:  core.Episode{Season: 1, Number: 1},
			},
			{
				FilePath: "/downloads/dungeon_02.mkv",
				Anime:    core.Anime{TitleCN: "迷宫饭", TitleEN: "Delicious in Dungeon", Type: "TV"},
				Episode:  core.Episode{Season: 1, Number: 2},
			},
		},
	}
	rawBatch, _ := json.Marshal(batchBody)
	reqBatch := httptest.NewRequest(http.MethodPost, "/api/organize/preview", bytes.NewReader(rawBatch))
	wBatch := httptest.NewRecorder()
	s.handlePreviewOrganize(wBatch, reqBatch)

	if wBatch.Code != http.StatusOK {
		t.Fatalf("批量预览期望 200, 实际 %d", wBatch.Code)
	}

	var batchResp struct {
		Code int                       `json:"code"`
		Data []organizer.PreviewResult `json:"data"`
	}
	if err := json.Unmarshal(wBatch.Body.Bytes(), &batchResp); err != nil {
		t.Fatalf("解析批量响应失败: %v", err)
	}
	if len(batchResp.Data) != 2 {
		t.Errorf("批量预览结果数量期望 2, 实际 %d", len(batchResp.Data))
	}
}

type mockTestDownloader struct {
	available bool
	name      string
}

func (m *mockTestDownloader) Name() string { return m.name }
func (m *mockTestDownloader) Add(ctx context.Context, item core.TorrentItem, savePath string) error {
	return nil
}
func (m *mockTestDownloader) List(ctx context.Context) ([]core.DownloadTask, error) {
	return nil, nil
}
func (m *mockTestDownloader) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	return core.DownloadTask{}, nil
}
func (m *mockTestDownloader) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	return nil
}
func (m *mockTestDownloader) Pause(ctx context.Context, hash string) error  { return nil }
func (m *mockTestDownloader) Resume(ctx context.Context, hash string) error { return nil }
func (m *mockTestDownloader) IsAvailable(ctx context.Context) bool          { return m.available }

func TestHandleTestDownloader(t *testing.T) {
	// 1. 测试未配置下载器
	sNoDL := &Server{downloader: nil}
	req1 := httptest.NewRequest(http.MethodPost, "/api/downloader/test", strings.NewReader(`{}`))
	w1 := httptest.NewRecorder()
	sNoDL.handleTestDownloader(w1, req1)
	if w1.Code != http.StatusOK || !strings.Contains(w1.Body.String(), `"success":false`) {
		t.Errorf("未配置下载器测试期望返回 success: false, 实际: %s", w1.Body.String())
	}

	// 2. 测试健康下载器
	sOK := &Server{
		downloader: &mockTestDownloader{available: true, name: "qBittorrent"},
	}
	req2 := httptest.NewRequest(http.MethodPost, "/api/downloader/test", strings.NewReader(`{}`))
	w2 := httptest.NewRecorder()
	sOK.handleTestDownloader(w2, req2)
	if w2.Code != http.StatusOK || !strings.Contains(w2.Body.String(), `"success":true`) {
		t.Errorf("健康下载器测试期望返回 success: true, 实际: %s", w2.Body.String())
	}

	// 3. 测试断网/不可用下载器
	sDown := &Server{
		downloader: &mockTestDownloader{available: false, name: "Transmission"},
	}
	req3 := httptest.NewRequest(http.MethodPost, "/api/downloader/test", strings.NewReader(`{}`))
	w3 := httptest.NewRecorder()
	sDown.handleTestDownloader(w3, req3)
	if w3.Code != http.StatusOK || !strings.Contains(w3.Body.String(), `"success":false`) {
		t.Errorf("断网下载器测试期望返回 success: false, 实际: %s", w3.Body.String())
	}
}





