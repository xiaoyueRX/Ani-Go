package backup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
	"gorm.io/gorm"
)

func TestBackupRestoreAndRotation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "library.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original; sqlDB, _ := db.DB(); sqlDB.Close() })
	if err = db.AutoMigrate(&database.Subscription{}, &database.Episode{}, &database.Setting{}); err != nil {
		t.Fatal(err)
	}
	m := &BackupManager{backupDir: filepath.Join(t.TempDir(), "backups"), keepCount: 1}
	var subs []database.Subscription
	for i, title := range []string{"第一部", "第二部"} {
		sub := database.Subscription{TitleCN: title, Season: 1}
		if err := db.Create(&sub).Error; err != nil {
			t.Fatal(err)
		}
		ep := database.Episode{SubscriptionID: sub.ID, Number: 1, TorrentHash: title}
		if err := db.Create(&ep).Error; err != nil {
			t.Fatal(err)
		}
		subs = append(subs, sub)
		if i == 0 {
			if err := db.Create(&database.Setting{Key: "test", Value: "before"}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx := context.Background()
	if err := m.CreateBackup(ctx, true); err != nil {
		t.Fatal(err)
	}
	files, err := m.ListBackups()
	if err != nil || len(files) != 1 {
		t.Fatalf("备份清单 %v %v", files, err)
	}
	name := files[0].Name
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{m.backupDir: 0700, filepath.Join(m.backupDir, name): 0600} {
			i, e := os.Stat(path)
			if e != nil || i.Mode().Perm() != want {
				t.Errorf("权限 %s: %v", path, i)
			}
		}
	}
	if err := db.Where("subscription_id = ?", subs[1].ID).Delete(&database.Episode{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&subs[1]).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreBackup(ctx, name); err != nil {
		t.Fatal(err)
	}
	for _, sub := range subs {
		var ep database.Episode
		var actual database.Subscription
		if err := db.First(&actual, sub.ID).Error; err != nil || actual.TitleCN != sub.TitleCN {
			t.Errorf("订阅未正确恢复 %d: %v", sub.ID, err)
		}
		if err := db.Where("subscription_id = ? AND torrent_hash = ?", sub.ID, sub.TitleCN).First(&ep).Error; err != nil {
			t.Errorf("剧集未正确恢复 %d: %v", sub.ID, err)
		}
	}
	if err := db.Model(&database.Setting{}).Where("key = ?", "test").Update("value", "after").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TRIGGER deny_updates BEFORE UPDATE ON episodes BEGIN SELECT RAISE(ABORT,'test rollback'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreBackup(ctx, name); err == nil {
		t.Error("写入失败被忽略")
	}
	var st database.Setting
	db.First(&st, "key = ?", "test")
	if st.Value != "after" {
		t.Error("写入失败未回滚设置")
	}
	unrelated := filepath.Join(m.backupDir, "other.json")
	if err := os.WriteFile(unrelated, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(ctx, true); err != nil {
		t.Fatal(err)
	}
	files, err = m.ListBackups()
	if err != nil || len(files) != 1 || files[0].Name == name {
		t.Errorf("应只保留新发布的备份: %v %v", files, err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Error("误删其他 JSON")
	}
}

func TestPluginCredentialsFollowBackupManifest(t *testing.T) {
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "archive.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original; sqlDB, _ := db.DB(); sqlDB.Close() })
	if err := db.AutoMigrate(&database.Subscription{}, &database.Episode{}, &database.Setting{}); err != nil {
		t.Fatal(err)
	}
	manager := &BackupManager{backupDir: filepath.Join(dir, "archives")}
	ctx := context.Background()
	save := func(key, value string) {
		t.Helper()
		if err := db.Save(&database.Setting{Key: key, Value: value}).Error; err != nil {
			t.Fatal("写入测试设置失败")
		}
	}
	read := func(key string) (string, bool) {
		t.Helper()
		var item database.Setting
		result := db.Where("key = ?", key).Limit(1).Find(&item)
		if result.Error != nil {
			t.Fatal("读取测试设置失败")
		}
		return item.Value, result.RowsAffected != 0
	}
	check := func(key, want string, present bool) {
		t.Helper()
		if got, ok := read(key); got != want || ok != present {
			t.Fatalf("恢复后的设置 %s 不符合预期", key)
		}
	}
	snapshot := func() (string, BackupData) {
		t.Helper()
		if err := manager.CreateBackup(ctx, false); err != nil {
			t.Fatal(err)
		}
		files, err := manager.ListBackups()
		if err != nil || len(files) != 1 {
			t.Fatal("生成测试备份失败")
		}
		content, err := os.ReadFile(manager.GetBackupPath(files[0].Name))
		var restored BackupData
		if err != nil || json.Unmarshal(content, &restored) != nil {
			t.Fatal("读取测试备份失败")
		}
		return files[0].Name, restored
	}
	name, empty := snapshot()
	if empty.Settings["custom_plugins"] != "[]" {
		t.Error("空实例生成的快照缺少显式插件清单")
	}
	if err := manager.DeleteBackup(name); err != nil {
		t.Fatal(err)
	}
	manifest := []plugin.PluginInfo{{ID: "observer", Name: "观察", Type: "webhook", URL: "https://example.org/v1", Secret: "private-signature", Panel: &plugin.Panel{
		Credential: &plugin.PanelCredential{Label: "访问令牌"}, Actions: []plugin.PanelAction{{ID: "list", Label: "列表", Result: "tasks"}},
	}}}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	keyFor := func() string {
		t.Helper()
		for key := range plugin.PanelCredentialSettingKeys(manifest) {
			return key
		}
		t.Fatal("缺少测试面板的授权键")
		return ""
	}
	key := keyFor()
	manifest[0].URL = "https://example.org/v2"
	otherKey := keyFor()
	builtin := plugin.PluginInfo{ID: "local-observer", IsBuiltIn: true, Panel: manifest[0].Panel}
	manager.SetBuiltinPanelManifests([]plugin.PluginInfo{builtin, manifest[0]})
	var builtinKey string
	for k := range plugin.PanelCredentialSettingKeys([]plugin.PluginInfo{builtin}) {
		builtinKey = k
	}
	if builtinKey == "" {
		t.Fatal("本地面板缺少测试凭据键")
	}
	save(builtinKey, "snapshot-local")
	save("custom_plugins", string(payload))
	save("plugin_status", `{"cloud-115":true,"observer":false}`)
	save(key, "snapshot-grant")
	save(otherKey, "retired-grant")
	save("CLOUD115_ACCESS_TOKEN", "snapshot-115")
	name, archive := snapshot()
	if archive.Settings[key] != "snapshot-grant" || archive.Settings[builtinKey] != "snapshot-local" || archive.Settings["CLOUD115_ACCESS_TOKEN"] != "snapshot-115" ||
		!strings.Contains(archive.Settings["custom_plugins"], "private-signature") {
		t.Error("快照未保存可达的插件授权")
	}
	if _, ok := archive.Settings[otherKey]; ok {
		t.Error("快照包含已退休目标的面板授权")
	}
	write := func(settings map[string]string) {
		t.Helper()
		raw, err := json.Marshal(BackupData{Timestamp: archive.Timestamp, Settings: settings})
		if err != nil || os.WriteFile(manager.GetBackupPath(name), raw, 0600) != nil {
			t.Fatal("写入测试快照失败")
		}
	}
	restore := func() {
		t.Helper()
		if err := manager.RestoreBackup(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	archive.Settings[otherKey] = "orphan-from-old-archive"
	write(archive.Settings)
	save(key, "replacement")
	save(builtinKey, "replacement-local")
	restore()
	check(key, "snapshot-grant", true)
	check(builtinKey, "snapshot-local", true)
	check(otherKey, "", false)
	save("plugin_status", `{"observer":true,"yuc_schedule":false}`)
	write(map[string]string{"plugin_status": `{"cloud-115":false,"observer":false}`, otherKey: "orphan-from-old-archive", "theme": "light"})
	restore()
	check(key, "snapshot-grant", true)
	check(otherKey, "", false)
	check("custom_plugins", string(payload), true)
	check("CLOUD115_ACCESS_TOKEN", "snapshot-115", true)
	check("theme", "light", true)
	rawStatus, _ := read("plugin_status")
	var states map[string]bool
	if json.Unmarshal([]byte(rawStatus), &states) != nil || !states["observer"] || states["cloud-115"] {
		t.Error("旧快照未区分内置开关与本机社区插件开关")
	}
	if _, ok := states["cloud-115"]; !ok {
		t.Error("旧快照中显式停用的内置开关被忽略")
	}
	for _, settings := range []map[string]string{
		{"custom_plugins": "null", "theme": "discard"},
		{"custom_plugins": "[{}]", "theme": "discard"},
		{"custom_plugins": "[]", "plugin_status": `{"observer":null}`, "theme": "discard"},
	} {
		write(settings)
		if err := manager.RestoreBackup(ctx, name); err == nil {
			t.Fatal("无效的插件快照不应写入数据库")
		}
		check(key, "snapshot-grant", true)
		check("theme", "light", true)
	}
	incomplete := BackupData{Timestamp: archive.Timestamp, Settings: map[string]string{"custom_plugins": "[]"},
		Episodes: []database.Episode{{SubscriptionID: 71, Title: "private-episode-fixture"}}}
	broken, err := json.Marshal(incomplete)
	if err != nil || os.WriteFile(manager.GetBackupPath(name), broken, 0600) != nil {
		t.Fatal("写入缺少订阅的测试快照失败")
	}
	if err := manager.RestoreBackup(ctx, name); err == nil || strings.Contains(err.Error(), "private-episode-fixture") {
		t.Error("无效剧集应使恢复失败且错误只含编号")
	}
	check(key, "snapshot-grant", true)
	check("CLOUD115_ACCESS_TOKEN", "snapshot-115", true)
	write(map[string]string{"custom_plugins": string(payload)})
	restore()
	check(key, "", false)
	check(builtinKey, "", false)
	check("CLOUD115_ACCESS_TOKEN", "", false)
}

func TestRestoreIntoOccupiedIDsAndUnrelatedLegacySource(t *testing.T) {
	dir := t.TempDir()
	open := func(name string) *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filepath.Join(dir, name)), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { sqlDB, _ := db.DB(); sqlDB.Close() })
		if err := db.AutoMigrate(&database.Subscription{}, &database.Episode{}, &database.Setting{}); err != nil {
			t.Fatal(err)
		}
		return db
	}
	original := database.DB
	defer func() { database.DB = original }()
	database.DB = open("source.db")
	manager := &BackupManager{backupDir: filepath.Join(dir, "snapshots")}
	sub := database.Subscription{BangumiID: "100", TitleCN: "来自备份", Season: 1}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	slot := "old-subscription-key"
	ep := database.Episode{SubscriptionID: sub.ID, Number: 2, TorrentHash: "shared-hash", ReservationKey: &slot}
	if err := database.DB.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.CreateBackup(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	files, err := manager.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	database.DB = open("destination.db")
	other := database.Subscription{BangumiID: "100", TitleCN: "另一个来源的作品", Season: 1}
	if err := database.DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	occupied := database.Episode{SubscriptionID: other.ID, Number: 1, TorrentHash: "another-hash"}
	if err := database.DB.Create(&occupied).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := manager.RestoreBackup(context.Background(), files[0].Name); err != nil {
			t.Fatal(err)
		}
	}
	var loaded database.Subscription
	if err := database.DB.First(&loaded, other.ID).Error; err != nil || loaded.TitleCN != other.TitleCN {
		t.Fatal("同号其他作品被覆盖", err)
	}
	var restored database.Episode
	if err := database.DB.Where("torrent_hash = ?", ep.TorrentHash).First(&restored).Error; err != nil || restored.SubscriptionID == other.ID || restored.ReservationKey != nil {
		t.Fatalf("跨库剧集未正确映射: %+v %v", restored, err)
	}
	id := restored.ID
	if err := database.DB.Unscoped().Delete(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.RestoreBackup(context.Background(), files[0].Name); err != nil {
		t.Fatal(err)
	}
	// 跨库导入时旧 ID 已被占用，必须分配其他编号。
	var count int64
	database.DB.Model(&database.Episode{}).Where("subscription_id = ?", restored.SubscriptionID).Count(&count)
	if id == occupied.ID || count != 1 {
		t.Fatal("原主键冲突后未恢复唯一剧集")
	}
}
