package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

func TestMigrationRequiresRunningPlugin(t *testing.T) {
	setupTestDB(t)
	t.Chdir(t.TempDir())
	if err := os.Mkdir("data", 0700); err != nil {
		t.Fatal(err)
	}
	const fixture = `[{"id":"s1","title":"Lifecycle fixture","url":"https://example.invalid/rss"}]`
	if err := os.WriteFile("data/ani.v2.json", []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	manager := plugin.NewManager(nil)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.StopAll)
	server := &Server{pluginManager: manager}
	importFixture := func(wantStatus int, wantCount int64) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/migrate", strings.NewReader(`{"source_path":"data/ani.v2.json"}`))
		response := httptest.NewRecorder()
		server.handleMigrateData(response, request)
		if response.Code != wantStatus {
			t.Fatalf("迁移状态码 %d，预期 %d，响应 %s", response.Code, wantStatus, response.Body)
		}
		var count int64
		if err := database.DB.Model(&database.Subscription{}).Count(&count).Error; err != nil || count != wantCount {
			t.Fatalf("订阅数 %d，预期 %d，错误 %v", count, wantCount, err)
		}
	}
	if state, _ := manager.GetPluginStatus("data-migrate"); state != plugin.StateSuspended {
		t.Fatalf("迁移插件应等待事件总线，实际 %s", state)
	}
	importFixture(http.StatusBadRequest, 0)

	manager.SetCoreService(core.ServiceEventBus, &dummyEventBus{})
	importFixture(http.StatusOK, 1)
	manager.GetPluginContext("data-migrate").Dispose()
	importFixture(http.StatusBadRequest, 1)
}

func TestToggleUnknownPluginReturnsNotFound(t *testing.T) {
	setupTestDB(t)
	manager := plugin.NewManager(&dummyEventBus{})
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.StopAll)
	server := &Server{pluginManager: manager}
	for _, path := range []string{"/api/plugins/toggle", "/api/plugins/missing/toggle"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"id":"missing","enabled":true}`))
		if path != "/api/plugins/toggle" {
			request.SetPathValue("id", "missing")
		}
		response := httptest.NewRecorder()
		server.handleTogglePlugin(response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("%s 返回 %d，预期 404", path, response.Code)
		}
	}
}
