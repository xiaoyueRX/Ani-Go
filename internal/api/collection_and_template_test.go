package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
	v2 "github.com/xiaoyueRX/Ani-Go/internal/notifier/v2"
)

func setupTestServerForNewFeatures(t *testing.T) (*Server, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}
	database.DB = db

	if err := db.AutoMigrate(&database.Subscription{}, &database.Episode{}, &database.Setting{}, &database.NotificationLog{}); err != nil {
		t.Fatalf("自动迁移表结构失败: %v", err)
	}

	bus := event.New()
	nm := v2.NewNotifyManager(nil, bus)

	s := &Server{
		notifyMgr: nm,
		version:   "v0.6.0",
		eventBus:  bus,
	}

	return s, db
}

func TestMergeCollectionsAPI(t *testing.T) {
	s, db := setupTestServerForNewFeatures(t)

	sub1 := database.Subscription{
		TitleCN:  "进击的巨人 最终季 完结篇 前篇",
		SeriesID: "",
	}
	db.Create(&sub1)

	sub2 := database.Subscription{
		TitleCN:  "进击的巨人 最终季 完结篇 后篇",
		SeriesID: "自定义进击",
	}
	db.Create(&sub2)

	// 1. 测试单条解析端点
	reqSingle := httptest.NewRequest(http.MethodPost, "/api/subscriptions/1/resolve-collection", nil)
	reqSingle.SetPathValue("id", "1")
	wSingle := httptest.NewRecorder()
	s.handleResolveSubscriptionCollection(wSingle, reqSingle)

	if wSingle.Code != http.StatusOK {
		t.Fatalf("handleResolveSubscriptionCollection 状态码错误: %d, body: %s", wSingle.Code, wSingle.Body.String())
	}

	var resSingle struct {
		Code int `json:"code"`
		Data struct {
			SeriesID string `json:"series_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(wSingle.Body).Decode(&resSingle); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resSingle.Data.SeriesID == "" {
		t.Errorf("解析结果系列名不能为空")
	}

	// 2. 测试批量预测计划 (Apply=false)
	bodyPreview, _ := json.Marshal(map[string]interface{}{
		"apply":     false,
		"overwrite": false,
	})
	reqPrev := httptest.NewRequest(http.MethodPost, "/api/subscriptions/merge-collections", bytes.NewReader(bodyPreview))
	wPrev := httptest.NewRecorder()
	s.handleMergeCollections(wPrev, reqPrev)

	if wPrev.Code != http.StatusOK {
		t.Fatalf("handleMergeCollections 预测失败: %d", wPrev.Code)
	}

	// 3. 测试批量落地执行 (Apply=true, Overwrite=true)
	bodyApply, _ := json.Marshal(map[string]interface{}{
		"apply":     true,
		"overwrite": true,
	})
	reqApply := httptest.NewRequest(http.MethodPost, "/api/subscriptions/merge-collections", bytes.NewReader(bodyApply))
	wApply := httptest.NewRecorder()
	s.handleMergeCollections(wApply, reqApply)

	if wApply.Code != http.StatusOK {
		t.Fatalf("handleMergeCollections 落地失败: %d", wApply.Code)
	}

	var resApply struct {
		Code int `json:"code"`
		Data struct {
			TotalApplied int `json:"total_applied"`
		} `json:"data"`
	}
	if err := json.NewDecoder(wApply.Body).Decode(&resApply); err != nil {
		t.Fatalf("解析落地结果失败: %v", err)
	}
	if resApply.Data.TotalApplied < 1 {
		t.Errorf("期望至少应用 1 条归并，实际: %d", resApply.Data.TotalApplied)
	}
}

func TestNotificationTemplatesAPI(t *testing.T) {
	s, _ := setupTestServerForNewFeatures(t)

	// 1. 获取模板列表
	reqList := httptest.NewRequest(http.MethodGet, "/api/notifications/templates", nil)
	wList := httptest.NewRecorder()
	s.handleListNotificationTemplates(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("获取模板列表失败: %d", wList.Code)
	}

	// 2. 预览有效模板
	previewBody, _ := json.Marshal(map[string]interface{}{
		"event_type": core.EventDownloadCompleted,
		"channel":    "telegram",
		"template":   "【自定义完成】{{.AnimeTitle}} 第 {{.Episode}} 集下载完成",
		"sample_data": map[string]interface{}{
			"anime_title": "葬送的芙莉莲",
			"episode":     12,
		},
	})
	reqPrev := httptest.NewRequest(http.MethodPost, "/api/notifications/templates/preview", bytes.NewReader(previewBody))
	wPrev := httptest.NewRecorder()
	s.handlePreviewNotificationTemplate(wPrev, reqPrev)

	if wPrev.Code != http.StatusOK {
		t.Fatalf("模板预览接口失败: %d, body: %s", wPrev.Code, wPrev.Body.String())
	}

	// 3. 预览语法错误模板应报错
	badPreviewBody, _ := json.Marshal(map[string]interface{}{
		"event_type": core.EventDownloadCompleted,
		"template":   "{{.Unclosed",
	})
	reqBadPrev := httptest.NewRequest(http.MethodPost, "/api/notifications/templates/preview", bytes.NewReader(badPreviewBody))
	wBadPrev := httptest.NewRecorder()
	s.handlePreviewNotificationTemplate(wBadPrev, reqBadPrev)

	if wBadPrev.Code != http.StatusBadRequest {
		t.Errorf("语法错误模板应返回 400, 实际: %d", wBadPrev.Code)
	}

	// 4. 更新模板
	updateBody, _ := json.Marshal(map[string]interface{}{
		"event_type": core.EventDownloadCompleted,
		"channel":    "wechat",
		"template":   "微信通知：{{.AnimeTitle}} 下载完成啦！",
	})
	reqUpdate := httptest.NewRequest(http.MethodPut, "/api/notifications/templates", bytes.NewReader(updateBody))
	wUpdate := httptest.NewRecorder()
	s.handleUpdateNotificationTemplate(wUpdate, reqUpdate)

	if wUpdate.Code != http.StatusOK {
		t.Fatalf("更新模板失败: %d, body: %s", wUpdate.Code, wUpdate.Body.String())
	}

	// 5. 重置模板
	resetBody, _ := json.Marshal(map[string]interface{}{
		"event_type": core.EventDownloadCompleted,
		"channel":    "wechat",
	})
	reqReset := httptest.NewRequest(http.MethodPost, "/api/notifications/templates/reset", bytes.NewReader(resetBody))
	wReset := httptest.NewRecorder()
	s.handleResetNotificationTemplate(wReset, reqReset)

	if wReset.Code != http.StatusOK {
		t.Fatalf("重置模板失败: %d, body: %s", wReset.Code, wReset.Body.String())
	}
}

func TestTestExtendedNotifier_WeChatAndSignal(t *testing.T) {
	s, _ := setupTestServerForNewFeatures(t)
	ctx := context.Background()

	// 1. 测试未配置参数时应返回清晰的未配置错误，而不是 panic
	errWeChat := s.testExtendedNotifier(ctx, "wechat", "测试标题", "测试内容")
	if errWeChat == nil {
		t.Errorf("未配置微信公众号参数时应报错")
	}

	errSignal := s.testExtendedNotifier(ctx, "signal", "测试标题", "测试内容")
	if errSignal == nil {
		t.Errorf("未配置 Signal 参数时应报错")
	}
}
