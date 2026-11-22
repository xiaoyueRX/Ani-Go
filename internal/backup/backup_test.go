package backup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func testBackupManager(t *testing.T) *BackupManager {
	t.Helper()
	old := database.DB
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	db, err := database.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); database.DB = old })
	return &BackupManager{backupDir: t.TempDir(), keepCount: 1}
}

func writeBackup(t *testing.T, m *BackupManager, backup BackupData) string {
	t.Helper()
	const name = "anigo_backup_20260101_000000_full.json"
	payload, err := json.Marshal(backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.backupDir, name), payload, 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestRestoreRemapsIDsAndCanBeRepeated(t *testing.T) {
	m := testBackupManager(t)
	local := database.Subscription{BangumiID: "local", TitleCN: "本地订阅"}
	if err := database.DB.Create(&local).Error; err != nil {
		t.Fatal(err)
	}
	imported := database.Subscription{BangumiID: "incoming", TitleCN: "导入订阅", Season: 1}
	imported.ID = local.ID
	ep := database.Episode{TorrentHash: "incoming-torrent", SubscriptionID: imported.ID, Number: 1, Season: 1, Status: "completed"}
	name := writeBackup(t, m, BackupData{Subscriptions: []database.Subscription{imported}, Episodes: []database.Episode{ep}})
	for i := 0; i < 2; i++ {
		if err := m.RestoreBackup(context.Background(), name); err != nil {
			t.Fatal(err)
		}
	}
	var parents []database.Subscription
	if err := database.DB.Find(&parents).Error; err != nil {
		t.Fatal(err)
	}
	if len(parents) != 2 {
		t.Fatalf("重复导入产生了额外订阅: %+v", parents)
	}
	var restored database.Episode
	if err := database.DB.Where("torrent_hash = ?", ep.TorrentHash).First(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if restored.SubscriptionID == local.ID || restored.ManualStatus != "completed" || restored.Status != "downloading" {
		t.Fatalf("恢复映射或状态错误: %+v", restored)
	}
}

func TestRestoreRollsBackOnEpisodeConflict(t *testing.T) {
	m := testBackupManager(t)
	local := database.Subscription{TitleCN: "本地订阅", BangumiID: "local"}
	if err := database.DB.Create(&local).Error; err != nil {
		t.Fatal(err)
	}
	existing := database.Episode{TorrentHash: "collision", SubscriptionID: local.ID}
	if err := database.DB.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	imported := database.Subscription{TitleCN: "新订阅", BangumiID: "imported", Season: 1}
	imported.ID = 7
	ep := database.Episode{SubscriptionID: imported.ID, TorrentHash: existing.TorrentHash}
	name := writeBackup(t, m, BackupData{Settings: map[string]string{"key": "imported"}, Subscriptions: []database.Subscription{imported}, Episodes: []database.Episode{ep}})
	if err := m.RestoreBackup(context.Background(), name); err == nil {
		t.Fatal("哈希归属冲突应回滚")
	}
	var count int64
	for _, item := range []struct {
		model    any
		expected int64
	}{{&database.Subscription{}, 1}, {&database.Setting{}, 0}, {&database.Episode{}, 1}} {
		if err := database.DB.Model(item.model).Count(&count).Error; err != nil || count != item.expected {
			t.Fatalf("回滚后的数据量异常: %T %d %v", item.model, count, err)
		}
	}
}

func TestRotateKeepsUnrelatedJSON(t *testing.T) {
	m := testBackupManager(t)
	const old = "anigo_backup_20260101_000000_full.json"
	for _, name := range []string{old, "external.json", "anigo_backup_notes.json"} {
		path := filepath.Join(m.backupDir, name)
		if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		past := time.Unix(1000, 0)
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.CreateBackup(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	m.cleanOldBackups()
	if _, err := os.Stat(filepath.Join(m.backupDir, old)); !os.IsNotExist(err) {
		t.Fatalf("过期备份未被轮转: %v", err)
	}
	for _, name := range []string{"external.json", "anigo_backup_notes.json"} {
		if _, err := os.Stat(filepath.Join(m.backupDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	files, err := m.ListBackups()
	if err != nil || len(files) != 1 {
		t.Fatalf("备份清单异常: %+v, %v", files, err)
	}
}

func TestRestoreKeepsSeparateFiltersOnSameSeries(t *testing.T) {
	m := testBackupManager(t)
	local := database.Subscription{TitleCN: "同一部番", BangumiID: "same", Season: 1, FilterJSON: "1080p"}
	if err := database.DB.Create(&local).Error; err != nil {
		t.Fatal(err)
	}
	imported := local
	imported.FilterJSON = "720p"
	ep := database.Episode{TorrentHash: "second-filter", SubscriptionID: imported.ID}
	name := writeBackup(t, m, BackupData{Subscriptions: []database.Subscription{imported}, Episodes: []database.Episode{ep}})
	for i := 0; i < 2; i++ {
		if err := m.RestoreBackup(context.Background(), name); err != nil {
			t.Fatal(err)
		}
	}
	var parents []database.Subscription
	if err := database.DB.Order("id").Find(&parents).Error; err != nil {
		t.Fatal(err)
	}
	if len(parents) != 2 || parents[0].FilterJSON != local.FilterJSON || parents[1].FilterJSON != imported.FilterJSON {
		t.Fatalf("筛选配置被覆盖: %+v", parents)
	}
	var episode database.Episode
	if err := database.DB.Where("torrent_hash = ?", ep.TorrentHash).First(&episode).Error; err != nil || episode.SubscriptionID != parents[1].ID {
		t.Fatalf("剧集指向错误的订阅: %+v %v", episode, err)
	}
}

func TestRestoreEpisodesWithoutHash(t *testing.T) {
	m := testBackupManager(t)
	parent := database.Subscription{TitleCN: "老订阅", Season: 1}
	parent.ID = 31
	backup := BackupData{Subscriptions: []database.Subscription{parent}}
	for _, number := range []float32{1, 2} {
		backup.Episodes = append(backup.Episodes, database.Episode{Number: number, Season: 1, SubscriptionID: parent.ID})
	}
	name := writeBackup(t, m, backup)
	for i := 0; i < 2; i++ {
		if err := m.RestoreBackup(context.Background(), name); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := database.DB.Model(&database.Episode{}).Where("torrent_hash IS NULL").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("无哈希剧集数量异常: %d, %v", count, err)
	}
}
