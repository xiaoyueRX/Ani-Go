package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

func TestSearchActionRequiresDeclaredMagnetAndCurrentTarget(t *testing.T) {
	setupTestDB(t)
	m := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(m.StopAll)
	mux := http.NewServeMux()
	(&Server{pluginManager: m}).registerRoutes(mux)
	const manifest = `{"id":"search-provider","name":"测试接收方","type":"webhook","url":"https://panel.example/search","panel":{"actions":[{"id":"enqueue","label":"提交","placement":"search_result","input":{"key":"magnet","label":"磁力","format":"magnet"},"result":"message"}]}}`
	save := "/api/plugins/save"
	panelCall(t, mux, "POST", save, manifest, 200)
	for _, invalid := range []string{
		strings.Replace(manifest, "search_result", "unknown", 1),
		strings.Replace(manifest, `"format":"magnet"`, `"format":""`, 1),
		strings.Replace(manifest, `"result":"message"`, `"result":"tasks"`, 1),
	} {
		panelCall(t, mux, "POST", save, invalid, 400)
	}
	const input = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"
	const target = "https://panel.example/search"
	call := func(body map[string]any, code int) {
		t.Helper()
		bytes, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		panelCall(t, mux, "POST", "/api/plugins/search-provider/panel/actions/enqueue", string(bytes), code)
	}
	installRunner := func() *int {
		t.Helper()
		_, scope, ok := m.PanelSnapshot("search-provider")
		if !ok || scope == nil {
			t.Fatal("缺少活动实例")
		}
		calls := 0
		scope.SetService(plugin.PanelService, panelRunFunc(func(ctx context.Context, action plugin.PanelAction, token, value string, page int) (plugin.PanelResult, error) {
			if value != input {
				t.Errorf("实际收到 %q", value)
			}
			calls++
			return plugin.PanelResult{Message: "已受理"}, nil
		}))
		return &calls
	}
	calls := installRunner()
	call(map[string]any{"input": input, "placement": "search_result", "expected_url": target}, 200)
	call(map[string]any{"input": input, "placement": "search_result"}, 400)
	call(map[string]any{"input": input, "placement": "elsewhere", "expected_url": target}, 400)
	call(map[string]any{"input": input, "placement": "search_result", "expected_url": "https://moved.example/"}, 409)
	call(map[string]any{"input": input}, 200)
	if *calls != 2 {
		t.Fatalf("实际投递 %d 次", *calls)
	}
	panelCall(t, mux, "POST", save, strings.Replace(manifest, `"placement":"search_result",`, "", 1), 200)
	calls = installRunner()
	call(map[string]any{"input": input, "placement": "search_result", "expected_url": target}, 409)
	if *calls != 0 {
		t.Fatal("已撤回的搜索操作仍然执行")
	}
}
