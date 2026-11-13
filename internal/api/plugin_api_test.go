package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

type dummyEventBus struct{}

func (d *dummyEventBus) Publish(event core.Event) {}
func (d *dummyEventBus) Subscribe(eventType string, handler core.EventHandler) core.SubscriptionID {
	return 1
}
func (d *dummyEventBus) Unsubscribe(eventType string, id core.SubscriptionID) {}

func TestPluginAPIs_NilManagerSafety(t *testing.T) {
	s := &Server{pluginManager: nil}

	// 1. handleListPlugins should return 200 [] without panic
	req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	w := httptest.NewRecorder()
	s.handleListPlugins(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("期望状态码 200, 实际: %d", w.Code)
	}

	// 2. handleGetCustomPlugins should return 200 [] without panic
	req = httptest.NewRequest(http.MethodGet, "/api/plugins/custom", nil)
	w = httptest.NewRecorder()
	s.handleGetCustomPlugins(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("期望状态码 200, 实际: %d", w.Code)
	}

	// 3. handleTogglePlugin should return 503
	body, _ := json.Marshal(map[string]interface{}{"id": "test", "enabled": true})
	req = httptest.NewRequest(http.MethodPost, "/api/plugins/toggle", bytes.NewReader(body))
	w = httptest.NewRecorder()
	s.handleTogglePlugin(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("期望状态码 503, 实际: %d", w.Code)
	}

	// 4. handleSavePlugin should return 503
	req = httptest.NewRequest(http.MethodPost, "/api/plugins/save", bytes.NewReader(body))
	w = httptest.NewRecorder()
	s.handleSavePlugin(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("期望状态码 503, 实际: %d", w.Code)
	}

	// 5. handleDeletePlugin should return 503
	req = httptest.NewRequest(http.MethodDelete, "/api/plugins/test", nil)
	w = httptest.NewRecorder()
	s.handleDeletePlugin(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("期望状态码 503, 实际: %d", w.Code)
	}

	// 6. handleReloadPlugins should return 503
	req = httptest.NewRequest(http.MethodPost, "/api/plugins/reload", nil)
	w = httptest.NewRecorder()
	s.handleReloadPlugins(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("期望状态码 503, 实际: %d", w.Code)
	}
}

func TestPluginAPIs_CRUDAndToggle(t *testing.T) {
	setupTestDB(t)
	bus := &dummyEventBus{}
	pm := plugin.NewManager(bus)
	pm.Load()
	s := &Server{pluginManager: pm}

	// 1. GET /api/plugins 返回内置插件
	req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	w := httptest.NewRecorder()
	s.handleListPlugins(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/plugins 失败: %d", w.Code)
	}
	var plugins []plugin.PluginInfo
	if err := json.NewDecoder(w.Body).Decode(&plugins); err != nil {
		t.Fatalf("解析插件列表失败: %v", err)
	}
	if len(plugins) == 0 {
		t.Fatalf("插件列表不应为空")
	}

	// 2. POST /api/plugins/save 创建自定义插件 (带非法 URL 应报错)
	badPlugin := plugin.PluginInfo{
		Name: "Test Webhook",
		Type: "webhook",
		URL:  "ftp://invalid-schema.com",
	}
	badBody, _ := json.Marshal(badPlugin)
	req = httptest.NewRequest(http.MethodPost, "/api/plugins/save", bytes.NewReader(badBody))
	w = httptest.NewRecorder()
	s.handleSavePlugin(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("非法 URL 预期 500, 实际: %d", w.Code)
	}

	// 创建合法自定义插件
	customPlugin := plugin.PluginInfo{
		ID:     "custom-test-1",
		Name:   "Discord Notify",
		Type:   "webhook",
		URL:    "https://discord.com/api/webhooks/test",
		Events: []string{"download.completed"},
	}
	body, _ := json.Marshal(customPlugin)
	req = httptest.NewRequest(http.MethodPost, "/api/plugins/save", bytes.NewReader(body))
	w = httptest.NewRecorder()
	s.handleSavePlugin(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("保存自定义插件失败: %d, %s", w.Code, w.Body.String())
	}

	// 3. GET /api/plugins/custom 仅返回自定义插件
	req = httptest.NewRequest(http.MethodGet, "/api/plugins/custom", nil)
	w = httptest.NewRecorder()
	s.handleGetCustomPlugins(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/plugins/custom 失败: %d", w.Code)
	}
	var customList []plugin.PluginInfo
	if err := json.NewDecoder(w.Body).Decode(&customList); err != nil {
		t.Fatalf("解析自定义列表失败: %v", err)
	}
	if len(customList) != 1 || customList[0].ID != "custom-test-1" {
		t.Fatalf("自定义列表结果不符合预期: %+v", customList)
	}

	// 4. POST /api/plugins/toggle 标准体切换停用
	disabled := false
	toggleBody, _ := json.Marshal(map[string]interface{}{
		"id":      "custom-test-1",
		"enabled": &disabled,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/plugins/toggle", bytes.NewReader(toggleBody))
	w = httptest.NewRecorder()
	s.handleTogglePlugin(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("停用插件失败: %d, %s", w.Code, w.Body.String())
	}
	if pm.IsPluginEnabled("custom-test-1") {
		t.Fatalf("插件应该已被停用")
	}

	// 5. POST /api/plugins/{id}/toggle 路径参数支持重新启用
	enabled := true
	togglePathBody, _ := json.Marshal(map[string]interface{}{
		"enabled": &enabled,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/plugins/custom-test-1/toggle", bytes.NewReader(togglePathBody))
	req.SetPathValue("id", "custom-test-1")
	w = httptest.NewRecorder()
	s.handleTogglePlugin(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("路径参数启用插件失败: %d, %s", w.Code, w.Body.String())
	}
	if !pm.IsPluginEnabled("custom-test-1") {
		t.Fatalf("插件应该已被启用")
	}

	// 6. DELETE /api/plugins/{id} 删除插件
	req = httptest.NewRequest(http.MethodDelete, "/api/plugins/custom-test-1", nil)
	req.SetPathValue("id", "custom-test-1")
	w = httptest.NewRecorder()
	s.handleDeletePlugin(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("删除插件失败: %d, %s", w.Code, w.Body.String())
	}

	// 确认已删除
	req = httptest.NewRequest(http.MethodGet, "/api/plugins/custom", nil)
	w = httptest.NewRecorder()
	s.handleGetCustomPlugins(w, req)
	var afterDelete []plugin.PluginInfo
	_ = json.NewDecoder(w.Body).Decode(&afterDelete)
	if len(afterDelete) != 0 {
		t.Fatalf("删除后自定义列表应为空，实际: %d", len(afterDelete))
	}
}

func TestPluginAPIs_ConcurrentReloadSafety(t *testing.T) {
	setupTestDB(t)
	bus := &dummyEventBus{}
	pm := plugin.NewManager(bus)
	pm.Load()
	s := &Server{pluginManager: pm}

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				req := httptest.NewRequest(http.MethodPost, "/api/plugins/reload", nil)
				w := httptest.NewRecorder()
				s.handleReloadPlugins(w, req)
				if w.Code != http.StatusOK {
					t.Errorf("并发重载失败: code=%d", w.Code)
				}
			} else {
				en := (idx%4 == 0)
				body, _ := json.Marshal(map[string]interface{}{
					"id":      "extended-notifiers",
					"enabled": en,
				})
				req := httptest.NewRequest(http.MethodPost, "/api/plugins/toggle", bytes.NewReader(body))
				w := httptest.NewRecorder()
				s.handleTogglePlugin(w, req)
				if w.Code != http.StatusOK {
					t.Errorf("并发状态切换失败: code=%d", w.Code)
				}
			}
		}(i)
	}

	wg.Wait()
}
