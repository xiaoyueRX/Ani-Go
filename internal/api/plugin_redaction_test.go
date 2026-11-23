package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

func TestPluginManagementKeepsWebhookSecretPrivate(t *testing.T) {
	setupTestDB(t)
	mgr := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(mgr.StopAll)
	mux := http.NewServeMux()
	(&Server{pluginManager: mgr}).registerRoutes(mux)
	const secret = "fixture-webhook-signing-value"
	request := func(method, path, body string) string {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s 返回 %d", method, path, w.Code)
		}
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("%s %s 返回了测试密钥", method, path)
		}
		return w.Body.String()
	}
	save := func(url string, key *string) {
		t.Helper()
		p := map[string]any{"id": "fixture-hook", "type": "webhook", "name": "测试通知", "url": url}
		if key != nil {
			p["secret"] = *key
		}
		body, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		request("POST", "/api/plugins/save", string(body))
	}
	checkStored := func(want string) {
		t.Helper()
		var stored database.Setting
		if err := database.DB.First(&stored, "key = ?", "custom_plugins").Error; err != nil {
			t.Fatal(err)
		}
		var items []plugin.PluginInfo
		if err := json.Unmarshal([]byte(stored.Value), &items); err != nil || len(items) != 1 || items[0].Secret != want {
			t.Fatal("持久化的 Webhook 密钥与预期不符")
		}
	}
	const original = "https://example.org/events/anime"
	key := secret
	save(original, &key)
	for _, path := range []string{"/api/plugins", "/api/plugins/custom", "/api/settings"} {
		request("GET", path, "")
	}
	request("POST", "/api/plugins/reload", "")
	save(original, nil)
	checkStored(secret)
	empty := ""
	save(original, &empty)
	checkStored("")
	save(original, &key)
	save(original+"/changed", nil)
	checkStored("")
}

func TestGeneralSettingsDoNotManagePluginState(t *testing.T) {
	setupTestDB(t)
	protected := map[string]string{
		"custom_plugins":              "[]",
		"plugin_status":               "{}",
		"PLUGIN_PANEL_CREDENTIAL_abc": "fixture-panel-value",
		"CLOUD115_ACCESS_TOKEN":       "fixture-cloud-value",
	}
	for key, value := range protected {
		if err := database.DB.Create(&database.Setting{Key: key, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{}
	w := httptest.NewRecorder()
	s.handleGetSettings(w, httptest.NewRequest("GET", "/api/settings", nil))
	var values map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &values); err != nil {
		t.Fatal(err)
	}
	for key := range protected {
		if key == "CLOUD115_ACCESS_TOKEN" {
			if value, ok := values[key]; !ok || value != "" {
				t.Error("115 令牌未按兼容接口脱敏")
			}
		} else if _, ok := values[key]; ok {
			t.Errorf("通用设置返回了插件内部字段 %s", key)
		}
	}
	updates := map[string]string{"theme": "dark"}
	for key := range protected {
		updates[key] = "updated"
		updates[strings.ToLower(key)] = "updated"
	}
	body, err := json.Marshal(settingsRequest{Settings: updates})
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	s.handleUpdateSettings(w, httptest.NewRequest("PUT", "/api/settings", strings.NewReader(string(body))))
	if w.Code != http.StatusOK {
		t.Fatalf("保存普通设置返回 %d", w.Code)
	}
	protected["theme"] = "dark"
	var stored []database.Setting
	if err := database.DB.Find(&stored).Error; err != nil || len(stored) != len(protected) {
		t.Fatal("通用设置创建了意外的字段")
	}
	for _, row := range stored {
		if row.Value != protected[row.Key] {
			t.Errorf("通用设置写入后的 %s 值与预期不符", row.Key)
		}
	}
}

func TestSavePluginClassifiesManifestErrors(t *testing.T) {
	setupTestDB(t)
	mgr := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(mgr.StopAll)
	mux := http.NewServeMux()
	(&Server{pluginManager: mgr}).registerRoutes(mux)
	save := func(body string, status int) {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/plugins/save", strings.NewReader(body)))
		if w.Code != status {
			t.Errorf("保存插件返回 %d，预期 %d", w.Code, status)
		}
		if strings.Contains(w.Body.String(), "fixture-private-value") {
			t.Error("保存插件的错误响应包含请求中的私密值")
		}
	}
	save(`{"name":"测试","url":"ftp://example.org/hooks","secret":"fixture-private-value"}`, http.StatusBadRequest)
	save(`{"name":"测试","url":"https://example.org/hooks","panel":{"version":2,"actions":[]}}`, http.StatusBadRequest)
	save(`{"name":"测试","url":"https://example.org/hooks","panel":{"actions":[]}}`, http.StatusBadRequest)
	save(`{"name":"测试","url":"https://example.org/hooks","id":"cloud-115"}`, http.StatusConflict)
	if err := database.DB.Create(&database.Setting{Key: "custom_plugins", Value: "损坏的配置"}).Error; err != nil {
		t.Fatal(err)
	}
	save(`{"name":"测试","url":"https://example.org/hooks","secret":"fixture-private-value"}`, http.StatusInternalServerError)
}
