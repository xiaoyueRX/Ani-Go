package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

type panelRunFunc func(context.Context, plugin.PanelAction, string, string, int) (plugin.PanelResult, error)

func (f panelRunFunc) Run(ctx context.Context, action plugin.PanelAction, token, input string, page int) (plugin.PanelResult, error) {
	return f(ctx, action, token, input, page)
}

func panelCall(t *testing.T, h http.Handler, method, path, body string, expected int) string {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	if w.Code != expected {
		t.Fatalf("%s %s: %d, want %d, %s", method, path, w.Code, expected, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "panel-private-") {
		t.Fatalf("%s %s 返回了测试凭据", method, path)
	}
	return w.Body.String()
}

func testPanelManifest(id string) plugin.PluginInfo {
	return plugin.PluginInfo{ID: id, Name: id, Type: "webhook", URL: "https://panel.example/action", Panel: &plugin.Panel{
		Credential: &plugin.PanelCredential{Label: "令牌", Required: true},
		Actions: []plugin.PanelAction{
			{ID: "send", Label: "提交", Input: &plugin.PanelInput{Key: "text", Label: "文本"}, Result: "message"},
			{ID: "list", Label: "分页", Result: "tasks"},
		},
	}}
}

func TestPanelActionsAndCredentialLifecycle(t *testing.T) {
	setupTestDB(t)
	m := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(m.StopAll)
	mux := http.NewServeMux()
	(&Server{pluginManager: m}).registerRoutes(mux)
	p := testPanelManifest("first")
	other := testPanelManifest("second")
	for _, manifest := range []plugin.PluginInfo{p, other} {
		if err := m.AddOrUpdatePlugin(manifest); err != nil {
			t.Fatal(err)
		}
	}
	const root = "/api/plugins/first/panel/"
	const cred = root + "credential"
	const send = root + "actions/send"
	const update = `{"access_token":"panel-private-example"}`
	panelCall(t, auth.AuthMiddleware(mux), "GET", cred, "", 401)
	panelCall(t, mux, "POST", send, `{"input":"hi"}`, 409)
	panelCall(t, mux, "POST", root+"actions/undeclared", "{}", 404)
	for _, params := range []string{`{}`, `{"input":"hi","page":1}`, `{"input":"hi","url":"extra"}`, `{"input":"hi"}{}`} {
		panelCall(t, mux, "POST", send, params, 400)
	}
	panelCall(t, mux, "POST", root+"actions/list", `{"page":0}`, 400)
	panelCall(t, mux, "PUT", cred, update, 200)
	panelCall(t, mux, "HEAD", cred, `{"access_token":"ignore"}`, 200)
	if got := panelCall(t, mux, "GET", "/api/plugins/second/panel/credential", "", 200); !strings.Contains(got, `"source":"none"`) {
		t.Fatalf("其他插件继承了凭据: %s", got)
	}

	_, scope, _ := m.PanelSnapshot(p.ID)
	calls := 0
	scope.SetService(plugin.PanelService, panelRunFunc(func(_ context.Context, act plugin.PanelAction, token, input string, page int) (plugin.PanelResult, error) {
		calls++
		if act.ID != "send" || token != "panel-private-example" || input != "hi" || page != 1 {
			t.Error("插件未获得自己的参数或凭据")
		}
		return plugin.PanelResult{Message: "accepted"}, nil
	}))
	panelCall(t, mux, "POST", send, `{"input":"hi"}`, 200)
	if calls != 1 {
		t.Fatal("操作运行次数错误")
	}

	for _, tc := range []struct {
		upstream error
		status   int
	}{
		{plugin.ErrPanelInput, 400},
		{plugin.ErrPanelUnauthorized, 422},
		{plugin.ErrPanelRateLimited, 429},
		{errors.New("upstream"), 502},
	} {
		scope.SetService(plugin.PanelService, panelRunFunc(func(_ context.Context, _ plugin.PanelAction, token, _ string, _ int) (plugin.PanelResult, error) {
			return plugin.PanelResult{}, errors.Join(tc.upstream, errors.New("response with "+token))
		}))
		panelCall(t, mux, "POST", send, `{"input":"hi"}`, tc.status)
	}

	previousURL := p.URL
	p.URL += "?account=second"
	if err := m.AddOrUpdatePlugin(p); err != nil {
		t.Fatal(err)
	}
	if got := panelCall(t, mux, "GET", cred, "", 200); !strings.Contains(got, `"source":"none"`) {
		t.Fatal(got)
	}
	late := "panel-private-stale"
	if _, err := m.StoredPanelCredential(context.Background(), scope, &late); !errors.Is(err, plugin.ErrPanelInactive) {
		t.Fatalf("陈旧实例可以写凭据: %v", err)
	}
	p.URL = previousURL
	if err := m.AddOrUpdatePlugin(p); err != nil {
		t.Fatal(err)
	}
	if got := panelCall(t, mux, "GET", cred, "", 200); !strings.Contains(got, `"source":"none"`) {
		t.Fatal("切回旧地址恢复了已清除的凭据")
	}
	panelCall(t, mux, "PUT", cred, update, 200)
	if err := m.DeletePlugin(p.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.AddOrUpdatePlugin(p); err != nil {
		t.Fatal(err)
	}
	if got := panelCall(t, mux, "GET", cred, "", 200); !strings.Contains(got, `"source":"none"`) {
		t.Fatal("重装后恢复了旧凭据")
	}

	panelCall(t, mux, "PUT", cred, update, 200)
	if err := database.DB.Exec("CREATE TRIGGER reject_manifest BEFORE UPDATE ON settings WHEN NEW.key='custom_plugins' BEGIN SELECT RAISE(ABORT, 'rejected'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := m.DeletePlugin(p.ID); err == nil {
		t.Fatal("测试事务没有按预期失败")
	}
	if got := panelCall(t, mux, "GET", cred, "", 200); !strings.Contains(got, `"source":"stored"`) {
		t.Fatal("失败的配置事务清除了仍在使用的凭据")
	}
}

func TestPanelUsesScopedRunnerAndCancelsWhenDisabled(t *testing.T) {
	setupTestDB(t)
	m := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(m.StopAll)
	p := testPanelManifest("cancel")
	p.Panel.Credential = nil
	if err := m.AddOrUpdatePlugin(p); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Server{pluginManager: m}).registerRoutes(mux)
	panelCall(t, mux, "GET", "/api/plugins/cancel/panel/credential", "", 404)
	_, scope, _ := m.PanelSnapshot(p.ID)
	started := make(chan struct{})
	scope.SetService(plugin.PanelService, panelRunFunc(func(ctx context.Context, _ plugin.PanelAction, token, _ string, _ int) (plugin.PanelResult, error) {
		if token != "" {
			t.Error("未声明凭据的操作收到令牌")
		}
		close(started)
		<-ctx.Done()
		return plugin.PanelResult{}, ctx.Err()
	}))
	done := make(chan int, 1)
	reqCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/plugins/cancel/panel/actions/send", strings.NewReader(`{"input":"ok"}`)).WithContext(reqCtx))
		done <- w.Code
	}()
	select {
	case <-started:
	case <-reqCtx.Done():
		t.Fatal("插件没有启动操作")
	}
	if err := m.Disable(p.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 409 {
			t.Fatalf("取消后状态码 %d", code)
		}
	case <-reqCtx.Done():
		t.Fatal("停用后操作未及时取消")
	}
}

func TestPanelCloud115CredentialCompatibility(t *testing.T) {
	setupTestDB(t)
	t.Setenv("CLOUD115_ACCESS_TOKEN", "")
	m := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(m.StopAll)
	if err := m.Enable(plugin.Cloud115ID); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Server{pluginManager: m}).registerRoutes(mux)
	const generic = "/api/plugins/cloud-115/panel/credential"
	const legacy = "/api/plugins/cloud-115/config"
	check := func(path, source string) {
		t.Helper()
		got := panelCall(t, mux, "GET", path, "", 200)
		var status plugin.PanelCredentialValue
		if json.Unmarshal([]byte(got), &status) != nil || status.Source != source || status.Configured != (source != "none") {
			t.Fatalf("配置状态 %s", got)
		}
	}
	panelCall(t, mux, "PUT", generic, `{"access_token":"panel-private-115"}`, 200)
	check(legacy, "stored")
	panelCall(t, mux, "DELETE", legacy, "", 200)
	check(generic, "none")
	panelCall(t, mux, "PUT", legacy, `{"access_token":"panel-private-existing"}`, 200)
	check(generic, "stored")
	t.Setenv("CLOUD115_ACCESS_TOKEN", "panel-private-env")
	check(generic, "environment")
	panelCall(t, mux, "DELETE", generic, "", 409)
	_, scope, _ := m.PanelSnapshot(plugin.Cloud115ID)
	scope.SetService(plugin.PanelService, panelRunFunc(func(_ context.Context, action plugin.PanelAction, token, input string, page int) (plugin.PanelResult, error) {
		if action.ID != "tasks" || token != "panel-private-env" || input != "" || page != 3 {
			t.Error("通用 115 操作没有使用环境提供的凭据或指定页码")
		}
		return plugin.PanelResult{Page: page, PageCount: 0, Tasks: []plugin.PanelTask{}}, nil
	}))
	result := panelCall(t, mux, "POST", "/api/plugins/cloud-115/panel/actions/tasks", `{"page":3}`, 200)
	if !strings.Contains(result, `"tasks":[]`) || !strings.Contains(result, `"page_count":0`) {
		t.Fatalf("空页结构不完整: %s", result)
	}

}

func TestPanelChecksReturnedResultBeforeExposingIt(t *testing.T) {
	setupTestDB(t)
	m := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(m.StopAll)
	mux := http.NewServeMux()
	(&Server{pluginManager: m}).registerRoutes(mux)
	p := testPanelManifest("result-contract")
	if err := m.AddOrUpdatePlugin(p); err != nil {
		t.Fatal(err)
	}
	const path = "/api/plugins/result-contract/panel/"
	panelCall(t, mux, "PUT", path+"credential", `{"access_token":"fixture-service-token"}`, 200)
	_, scope, _ := m.PanelSnapshot(p.ID)
	for _, tc := range []struct {
		action, body string
		result       plugin.PanelResult
	}{
		{"send", `{"input":"one"}`, plugin.PanelResult{}},
		{"send", `{"input":"one"}`, plugin.PanelResult{Message: "fixture-service-token"}},
		{"send", `{"input":"one"}`, plugin.PanelResult{Message: "ok", Page: 1, Tasks: []plugin.PanelTask{}}},
		{"list", "{}", plugin.PanelResult{Page: 1, Tasks: []plugin.PanelTask{{Name: "complete", InfoHash: "fixture", Progress: 130, Status: "completed"}}}},
		{"list", "{}", plugin.PanelResult{Message: "not a page"}},
	} {
		scope.SetService(plugin.PanelService, panelRunFunc(func(context.Context, plugin.PanelAction, string, string, int) (plugin.PanelResult, error) {
			return tc.result, nil
		}))
		body := panelCall(t, mux, "POST", path+"actions/"+tc.action, tc.body, 502)
		if strings.Contains(body, "fixture-service-token") {
			t.Fatal("公开了服务凭据")
		}
	}
}
