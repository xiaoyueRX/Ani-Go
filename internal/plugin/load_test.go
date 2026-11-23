package plugin

import (
	"errors"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	legacyPluginStatus = `{"data-migrate":false,"cloud-115":true,"my-hook":false}`
	legacyCustomList   = `[{"id":"cloud-115","name":"旧插件","type":"script"}]`
)

func storePluginConfig(t *testing.T, db *gorm.DB, key, value string) {
	t.Helper()
	if err := db.Save(&database.Setting{Key: key, Value: value}).Error; err != nil {
		t.Fatal(err)
	}
}

func checkPluginConfig(t *testing.T, db *gorm.DB, key, expected string) {
	t.Helper()
	var setting database.Setting
	if err := db.First(&setting, "key = ?", key).Error; err != nil {
		t.Fatal(err)
	}
	if setting.Value != expected {
		t.Errorf("%s 应保留 %q，实际 %q", key, expected, setting.Value)
	}
}

func TestPluginColdLoadDoesNotReplaceUnavailableStatus(t *testing.T) {
	db := setupTestDB(t)
	storePluginConfig(t, db, "plugin_status", legacyPluginStatus)
	storePluginConfig(t, db, "custom_plugins", legacyCustomList)
	failed := false
	if err := db.Callback().Query().Before("gorm:query").Register("temporary_status_read_failure", func(tx *gorm.DB) {
		where, _ := tx.Statement.Clauses["WHERE"].Expression.(clause.Where)
		for _, expr := range where.Exprs {
			if e, ok := expr.(clause.Expr); ok && len(e.Vars) == 1 && e.Vars[0] == "plugin_status" && !failed {
				failed = true
				tx.AddError(errors.New("database is locked"))
				return
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	m := NewManager(newTestBus())
	m.builtInPlugins = []BuiltInPlugin{provenanceBuiltin{PluginInfo{ID: "data-migrate"}}}
	defer m.StopAll()
	if err := m.Load(); err == nil {
		t.Fatal("加载失败未报告错误")
	}
	checkPluginConfig(t, db, "plugin_status", legacyPluginStatus)
	checkPluginConfig(t, db, "custom_plugins", legacyCustomList)
	if !failed || m.GetPluginContext("data-migrate") != nil || m.IsPluginEnabled("data-migrate") || len(m.customPlugins) != 0 {
		t.Fatalf("状态读取失败后应用了未经验证的配置")
	}
	for _, p := range m.List() {
		if p.Enabled || p.Status == StateActive {
			t.Errorf("列表误报已启用: %+v", p)
		}
	}
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	if len(m.customPlugins) != 1 || m.customPlugins[0].ID != "custom-cloud-115" || m.IsPluginEnabled("my-hook") || m.IsPluginEnabled("data-migrate") {
		t.Fatal("锁释放后未恢复正确的全部开关并完成迁移")
	}
}

func TestPluginReloadRejectsMalformedSettingsBeforeAnyMutation(t *testing.T) {
	for _, tc := range []struct{ name, states, plugins string }{
		{"状态损坏", "{", legacyCustomList},
		{"清单损坏", legacyPluginStatus, "["},
		{"状态缺少对象", "null", legacyCustomList},
		{"空值开关", `{"data-migrate":null}`, legacyCustomList},
		{"清单包含空插件", legacyPluginStatus, `[null]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			m := NewManager(newTestBus())
			m.builtInPlugins = []BuiltInPlugin{provenanceBuiltin{PluginInfo{ID: "data-migrate"}}}
			defer m.StopAll()
			storePluginConfig(t, db, "plugin_status", `{"data-migrate":true}`)
			storePluginConfig(t, db, "custom_plugins", `[{"id":"prior","type":"script"}]`)
			m.Load()
			prior := m.GetPluginContext("data-migrate")
			if prior == nil {
				t.Fatal("未加载正常配置")
			}
			storePluginConfig(t, db, "plugin_status", tc.states)
			storePluginConfig(t, db, "custom_plugins", tc.plugins)
			if err := m.Load(); err == nil {
				t.Error("损坏的配置应返回错误")
			}
			if m.GetPluginContext("data-migrate") != prior || !m.IsPluginEnabled("data-migrate") || len(m.customPlugins) != 1 || m.customPlugins[0].ID != "prior" {
				t.Error("热重载失败改变了先前生效的配置或上下文")
			}
			if err := m.Disable("data-migrate"); err == nil {
				t.Error("损坏时允许覆盖全部开关")
			}
			if err := m.AddOrUpdatePlugin(PluginInfo{ID: "extra", Name: "新增", Type: "script"}); err == nil {
				t.Error("损坏时允许覆盖全部插件")
			}
			if err := m.DeletePlugin("prior"); err == nil {
				t.Error("损坏时允许删除插件")
			}
			checkPluginConfig(t, db, "plugin_status", tc.states)
			checkPluginConfig(t, db, "custom_plugins", tc.plugins)
		})
	}
}

func TestPluginMigrationWriteFailureKeepsRuntimeAndDatabaseSnapshots(t *testing.T) {
	db := setupTestDB(t)
	storePluginConfig(t, db, "plugin_status", `{"data-migrate":true}`)
	storePluginConfig(t, db, "custom_plugins", `[{"id":"prior","type":"script"}]`)
	m := NewManager(newTestBus())
	m.builtInPlugins = []BuiltInPlugin{provenanceBuiltin{PluginInfo{ID: "data-migrate"}}}
	defer m.StopAll()
	m.Load()
	prior := m.GetPluginContext("data-migrate")
	storePluginConfig(t, db, "plugin_status", legacyPluginStatus)
	storePluginConfig(t, db, "custom_plugins", legacyCustomList)
	if err := db.Exec("CREATE TRIGGER reject_migration BEFORE UPDATE ON settings WHEN NEW.key='plugin_status' BEGIN SELECT RAISE(ABORT, 'migration write failed'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := m.Load(); err == nil {
		t.Fatal("加载失败未报告错误")
	}
	checkPluginConfig(t, db, "plugin_status", legacyPluginStatus)
	checkPluginConfig(t, db, "custom_plugins", legacyCustomList)
	if prior == nil || m.GetPluginContext("data-migrate") != prior || !m.IsPluginEnabled("data-migrate") || len(m.customPlugins) != 1 || m.customPlugins[0].ID != "prior" {
		t.Fatal("数据库拒绝写回时提前提交了内存快照")
	}
	if err := db.Exec("DROP TRIGGER reject_migration").Error; err != nil {
		t.Fatal(err)
	}
	m.Load()
	if m.GetPluginContext("data-migrate") != nil || len(m.customPlugins) != 1 || m.customPlugins[0].ID != "custom-cloud-115" {
		t.Fatal("写回恢复后未应用有效迁移")
	}
}

func TestPluginMissingStatusRowUsesFreshDefaultsAndCanMigrate(t *testing.T) {
	db := setupTestDB(t)
	storePluginConfig(t, db, "custom_plugins", legacyCustomList)
	m := NewManager(newTestBus())
	m.builtInPlugins = []BuiltInPlugin{provenanceBuiltin{PluginInfo{ID: "data-migrate"}}}
	defer m.StopAll()
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if m.GetPluginContext("data-migrate") == nil || len(m.customPlugins) != 1 || m.customPlugins[0].ID != "custom-cloud-115" {
		t.Fatal("实际缺少开关记录时应使用新安装默认值并完成迁移")
	}
	checkPluginConfig(t, db, "plugin_status", "{}")
}
