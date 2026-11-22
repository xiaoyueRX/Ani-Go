package database

import (
	"path/filepath"
	"testing"
)

// 旧格式的同一标记可能对应下载中的任务，也可能对应已经入库的剧集。
func TestUpgradeManualCompletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	oldDB, err := DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	for _, ep := range []Episode{
		{SubscriptionID: 1, TorrentHash: "active", Status: "completed"},
		{SubscriptionID: 1, TorrentHash: "finished", Status: "completed", FinalPath: "/media/1.mkv"},
	} {
		if err := DB.Create(&ep).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := oldDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	for hash, want := range map[string]string{"active": "downloading", "finished": "organized"} {
		var got Episode
		if err := DB.Where("torrent_hash = ?", hash).First(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got.Status != want || got.ManualStatus != "completed" {
			t.Fatalf("after upgrade %s: status=%s manual=%s", hash, got.Status, got.ManualStatus)
		}
	}
}
