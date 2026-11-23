package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/backup"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

func TestPluginSaveAndToggleReportCommittedState(t *testing.T) {
	setupTestDB(t)
	m := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(m.StopAll)
	mux := http.NewServeMux()
	(&Server{pluginManager: m}).registerRoutes(mux)
	manifest := testPanelManifest("")
	save := func() plugin.PluginInfo {
		t.Helper()
		body, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		result := panelCall(t, mux, "POST", "/api/plugins/save", string(body), 200)
		var info plugin.PluginInfo
		if err := json.Unmarshal([]byte(result), &info); err != nil {
			t.Fatal(err)
		}
		return info
	}
	manifest.Name = "自定义任务"
	saved := save()
	if saved.ID == "" || !saved.Enabled || saved.Status != plugin.StateActive {
		t.Fatalf("创建响应没有实际生成的 ID 和运行状态：%+v", saved)
	}
	toggle := "/api/plugins/" + saved.ID + "/toggle"
	for _, wrong := range []string{`{"enabled":"false"}`, `{"enabled":false}{}`} {
		panelCall(t, mux, "POST", toggle, wrong, 400)
		if !m.IsPluginActive(saved.ID) {
			t.Fatal("格式错误的开关请求改变了插件状态")
		}
	}
	panelCall(t, mux, "POST", toggle, `{"enabled":false}`, 200)
	manifest.ID = " " + saved.ID + " "
	edited := save()
	if edited.ID != saved.ID || edited.Enabled || edited.Status != plugin.StateInactive {
		t.Fatalf("编辑响应与已停用的插件不符：%+v", edited)
	}
	panelCall(t, mux, "DELETE", "/api/plugins/"+saved.ID, "", 200)
	panelCall(t, mux, "DELETE", "/api/plugins/"+saved.ID, "", 404)
}

func TestPluginRestoreUpdatesLivePanelAndCredentials(t *testing.T) {
	setupTestDB(t)
	m := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(m.StopAll)
	cfg := new(config.Config)
	cfg.Scheduler.BackupPath = t.TempDir()
	bm := backup.NewBackupManager(cfg)
	mux := http.NewServeMux()
	(&Server{pluginManager: m, backupManager: bm}).registerRoutes(mux)
	p := testPanelManifest("restore-example")
	if err := m.AddOrUpdatePlugin(p); err != nil {
		t.Fatal(err)
	}
	const cred = "/api/plugins/restore-example/panel/credential"
	panelCall(t, mux, "PUT", cred, `{"access_token":"panel-private-original"}`, 200)
	panelCall(t, mux, "POST", "/api/backup/create", "{}", 200)
	files, err := bm.ListBackups()
	if err != nil || len(files) != 1 {
		t.Fatal("备份未创建", err)
	}
	originalURL := p.URL
	p.URL += "?site=new"
	if err := m.AddOrUpdatePlugin(p); err != nil {
		t.Fatal(err)
	}
	panelCall(t, mux, "PUT", cred, `{"access_token":"panel-private-later"}`, 200)
	_, previous, _ := m.PanelSnapshot(p.ID)
	restore, _ := json.Marshal(map[string]string{"name": files[0].Name})
	panelCall(t, mux, "POST", "/api/backup/restore", string(restore), 200)
	info, scope, found := m.PanelSnapshot(p.ID)
	if !found || info.URL != originalURL || scope == nil || scope == previous || previous.Context().Err() == nil {
		t.Fatal("恢复成功后，旧面板或目标仍在运行")
	}
	if token, err := m.StoredPanelCredential(t.Context(), scope, nil); err != nil || token != "panel-private-original" {
		t.Fatal("恢复后的面板未绑定快照中的对应凭据", err)
	}
	if err := m.AddOrUpdatePlugin(p); err != nil {
		t.Fatal(err)
	}
	if got := panelCall(t, mux, "GET", cred, "", 200); !strings.Contains(got, `"source":"none"`) {
		t.Fatal("恢复后切至其他目标仍能读取已作废的凭据")
	}
}

func TestPanelTypedRequestsAndEmptyTaskPage(t *testing.T) {
	setupTestDB(t)
	t.Setenv("CLOUD115_ACCESS_TOKEN", "")
	m := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(m.StopAll)
	if err := m.Enable(plugin.Cloud115ID); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Server{pluginManager: m}).registerRoutes(mux)
	const root = "/api/plugins/cloud-115/panel/"
	for _, body := range []string{`{"access_token":"example","extra":true}`, `{"access_token":"example"}{}`} {
		panelCall(t, mux, "PUT", root+"credential", body, 400)
	}
	got := panelCall(t, mux, "GET", root+"credential", "", 200)
	if !strings.Contains(got, `"configured":false`) {
		t.Fatal("无效的通用凭据请求改写了设置")
	}
	panelCall(t, mux, "PUT", root+"credential", `{"access_token":"example"}`, 200)
	pages := 0
	_, scope, _ := m.PanelSnapshot(plugin.Cloud115ID)
	scope.SetService(plugin.PanelService, panelRunFunc(func(_ context.Context, _ plugin.PanelAction, _, _ string, page int) (plugin.PanelResult, error) {
		pages++
		return plugin.PanelResult{Page: page}, nil
	}))
	for _, body := range []string{"null", `{"input":null}`, `{"page":null}`, `{"page":"1"}`} {
		panelCall(t, mux, "POST", root+"actions/tasks", body, 400)
	}
	got = panelCall(t, mux, "POST", root+"actions/tasks", "{}", 200)
	if pages != 1 || !strings.Contains(got, `"tasks":[]`) || !strings.Contains(got, `"page_count":0`) {
		t.Fatalf("空任务页应返回完整分页结构，实际 %s，执行 %d 次", got, pages)
	}
}

func TestPanelCredentialProviderFollowsOwningScope(t *testing.T) {
	setupTestDB(t)
	const key = "ANIGO_TEST_OTHER_PANEL_TOKEN"
	t.Setenv(key, "")
	m := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(m.StopAll)
	for _, id := range []string{"alternate-provider", "ordinary-provider"} {
		if err := m.AddOrUpdatePlugin(testPanelManifest(id)); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	(&Server{pluginManager: m}).registerRoutes(mux)
	_, scope, _ := m.PanelSnapshot("alternate-provider")
	// 即使本地 Go 插件将服务同时发布到共享注册表，其他面板也只读取自己的声明。
	scope.RegisterService(plugin.PanelCredentialService, plugin.PanelCredentialStorage{EnvironmentKey: key})
	const root = "/api/plugins/alternate-provider/panel/"
	const cred = root + "credential"
	panelCall(t, mux, "PUT", cred, `{"access_token":"panel-private-separate"}`, 200)
	t.Setenv(key, "panel-private-environment")
	if got := panelCall(t, mux, "GET", cred, "", 200); !strings.Contains(got, `"source":"environment"`) {
		t.Fatal("插件私有环境来源未生效")
	}
	panelCall(t, mux, "PUT", cred, `{"access_token":"panel-private-overwrite"}`, 409)
	panelCall(t, mux, "DELETE", cred, "", 409)
	if got := panelCall(t, mux, "GET", "/api/plugins/ordinary-provider/panel/credential", "", 200); !strings.Contains(got, `"source":"none"`) {
		t.Fatal("另一插件读取了本插件的凭据声明")
	}
	want := "panel-private-environment"
	scope.SetService(plugin.PanelService, panelRunFunc(func(_ context.Context, _ plugin.PanelAction, token, _ string, _ int) (plugin.PanelResult, error) {
		if token != want {
			t.Error("执行器未获得自己当前生效的凭据")
		}
		return plugin.PanelResult{Message: "完成"}, nil
	}))
	panelCall(t, mux, "POST", root+"actions/send", `{"input":"工作"}`, 200)
	t.Setenv(key, "")
	want = "panel-private-separate"
	panelCall(t, mux, "POST", root+"actions/send", `{"input":"工作"}`, 200)
	panelCall(t, mux, "DELETE", cred, "", 200)
	panelCall(t, mux, "POST", root+"actions/send", `{"input":"工作"}`, 409)
}
