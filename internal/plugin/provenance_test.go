package plugin

import (
	"encoding/json"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

type provenanceBuiltin struct{ PluginInfo }

func (p provenanceBuiltin) GetInfo() PluginInfo                  { return p.PluginInfo }
func (provenanceBuiltin) Init(core.EventBus, core.Context) error { return nil }
func (provenanceBuiltin) Stop(core.EventBus) error               { return nil }

func TestPluginProvenanceComesFromRegistration(t *testing.T) {
	m := NewManager(newTestBus())
	m.builtInPlugins = append(m.builtInPlugins, provenanceBuiltin{PluginInfo{ID: "external-tool", Author: "External maintainer"}})
	wantOfficial := map[string]bool{
		"metadata-mapping": true, "standard-naming": true, "yuc_schedule": true,
		"data-migrate": true, "extended-notifiers": true, "nfo_scraper": true, "cloud-115": true,
	}
	for _, p := range m.List() {
		want := "community"
		if wantOfficial[p.ID] {
			want = "official"
			delete(wantOfficial, p.ID)
		}
		if p.Origin != want || !p.IsBuiltIn {
			t.Errorf("%s 归属 = %q，内置 = %v", p.ID, p.Origin, p.IsBuiltIn)
		}
		if p.ID == "external-tool" && p.Author != "External maintainer" {
			t.Errorf("作者发生变化: %q", p.Author)
		}
	}
	if len(wantOfficial) != 0 {
		t.Errorf("未注册的官方插件: %v", wantOfficial)
	}
}

func TestPluginProvenanceImportEditAndReservedIDs(t *testing.T) {
	setupTestDB(t)
	m := NewManager(newTestBus())
	m.builtInPlugins = []BuiltInPlugin{provenanceBuiltin{PluginInfo{ID: "external-tool"}}}
	defer m.StopAll()
	p := PluginInfo{ID: "community-hook", Name: "扩展", Type: "webhook", URL: "https://example.org/hook", Author: "xiaoyue", AuthorURL: "https://example.com/author", Origin: "official", IsBuiltIn: true}
	if err := m.AddOrUpdatePlugin(p); err != nil {
		t.Fatal(err)
	}
	if err := m.Disable(p.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.AddOrUpdatePlugin(PluginInfo{ID: p.ID, Name: "编辑名称", Type: "webhook", URL: p.URL}); err != nil {
		t.Fatal(err)
	}
	m.Reload()
	found := false
	for _, item := range m.List() {
		if item.ID != p.ID {
			continue
		}
		found = true
		if item.Origin != "community" || item.IsBuiltIn || item.Author != p.Author || item.AuthorURL != p.AuthorURL || item.Enabled || item.Name != "编辑名称" {
			t.Errorf("编辑并重载后的元数据错误: %+v", item)
		}
	}
	if !found {
		t.Fatal("自定义插件丢失")
	}
	for _, id := range []string{"CLOUD-115", " nfo_scraper ", "external-tool"} {
		p.ID = id
		if err := m.AddOrUpdatePlugin(p); err == nil {
			t.Errorf("接受了保留 ID %q", id)
		}
		if err := m.DeletePlugin(id); err == nil {
			t.Errorf("允许删除保留 ID %q", id)
		}
	}
}

func TestPluginProvenanceLegacyCollisionPersistsAcrossRestart(t *testing.T) {
	db := setupTestDB(t)
	save := func(k string, v any) {
		t.Helper()
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Save(&database.Setting{Key: k, Value: string(b)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	save("custom_plugins", []map[string]any{
		{"id": "cloud-115", "name": "存量冲突插件", "type": "script", "is_builtin": true, "origin": "official", "author": "原作者", "enabled": true},
		{"id": "custom-cloud-115", "name": "同名前缀自定义插件", "type": "script", "author": "另一作者"},
		{"id": "old-hook", "name": "旧版插件", "type": "script", "author": "xiaoyue", "enabled": true},
	})
	save("plugin_status", map[string]bool{"cloud-115": true, "old-hook": false})
	manager := func() *Manager {
		m := NewManager(newTestBus())
		m.builtInPlugins = []BuiltInPlugin{provenanceBuiltin{PluginInfo{ID: "cloud-115", Author: "xiaoyue"}}}
		m.Load()
		return m
	}
	m := manager()
	var newID string
	for _, p := range m.List() {
		switch p.Name {
		case "存量冲突插件":
			if p.ID == "cloud-115" || p.ID == "custom-cloud-115" || p.Origin != "community" || p.IsBuiltIn || p.Author != "原作者" || p.Enabled {
				t.Fatalf("冲突配置未安全保留: %+v", p)
			}
			newID = p.ID
		case "旧版插件":
			if p.ID != "old-hook" || p.Enabled || p.Origin != "community" || p.Author != "xiaoyue" {
				t.Errorf("旧插件元数据损坏: %+v", p)
			}
		}
	}
	if m.IsPluginEnabled(Cloud115ID) {
		t.Fatal("共享的旧开关启用了官方插件")
	}
	if newID == "" {
		t.Fatal("丢失已有的同名自定义插件")
	}
	if err := m.Enable(newID); err != nil {
		t.Fatal(err)
	}
	if err := m.DeletePlugin("custom-cloud-115"); err != nil {
		t.Fatal(err)
	}
	m.StopAll()
	again := manager()
	defer again.StopAll()
	for _, p := range again.List() {
		if p.Name == "存量冲突插件" && p.ID == newID && p.Enabled && !p.IsBuiltIn {
			return
		}
	}
	t.Fatal("重启后冲突插件的 ID 或启用状态丢失")
}
