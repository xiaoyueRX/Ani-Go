package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestUpgradeEpisodeHashAllowsSeasonPacks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"CREATE TABLE episodes (id integer PRIMARY KEY,subscription_id integer NOT NULL,season integer DEFAULT 1,number real NOT NULL,torrent_hash text)",
		"CREATE UNIQUE INDEX idx_episodes_torrent_hash ON episodes(torrent_hash)",
		"INSERT INTO episodes(id,subscription_id,season,number,torrent_hash) VALUES (37,2,1,1,'shared-hash')",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	old := DB
	t.Cleanup(func() {
		if conn, e := DB.DB(); e == nil {
			_ = conn.Close()
		}
		DB = old
	})
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	var ep Episode
	if err := DB.First(&ep, 37).Error; err != nil || ep.Number != 1 {
		t.Fatalf("旧剧集未保留: %+v %v", ep, err)
	}
	second := Episode{SubscriptionID: 2, Season: 0, Number: 2, TorrentHash: "shared-hash"}
	if err := DB.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.First(&second, second.ID).Error; err != nil || second.Season != 0 {
		t.Fatalf("特别篇季号错误: %+v %v", second, err)
	}
}

func TestMigrateManualEpisodeStatusAndProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.AutoMigrate(&Subscription{}, &Episode{}); err != nil {
		t.Fatal(err)
	}
	sub := Subscription{TitleCN: "迁移", Season: 1, TotalEpisodes: 6, CurrentEpisodes: 6, Completed: true}
	if err := legacy.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tests := []struct {
		ep          Episode
		want, label string
	}{
		{Episode{Status: "completed", FinalPath: "lib/01.mkv"}, "organized", "completed"},
		{Episode{Status: "completed", DownloadFinishedAt: &now}, "downloaded", "completed"},
		{Episode{Status: "completed", TorrentHash: "bound"}, "downloading", "completed"},
		{Episode{Status: "completed"}, "pending", "completed"},
		{Episode{Status: "failed", DownloadStartedAt: &now}, "downloading", "failed"},
		{Episode{Status: "pending", FinalPath: "lib/06.mkv"}, "organized", "pending"},
		{Episode{Status: "failed"}, "failed", ""},
		{Episode{Status: "downloading", DownloadStartedAt: &now}, "downloading", ""},
		{Episode{Status: "completed", FinalPath: "lib/recycled.mkv"}, "organized", "completed"},
	}
	for i := range tests {
		ep := &tests[i].ep
		ep.Season, ep.SubscriptionID, ep.Number = 1, sub.ID, float32(i+1)
		if err := legacy.Create(ep).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := legacy.Delete(&tests[len(tests)-1].ep).Error; err != nil {
		t.Fatal(err)
	}
	if err := legacy.Exec("ALTER TABLE episodes DROP COLUMN manual_status").Error; err != nil {
		t.Fatal(err)
	}
	legacyConn, _ := legacy.DB()
	if err := legacyConn.Close(); err != nil {
		t.Fatal(err)
	}
	old := DB
	t.Cleanup(func() { DB = old })
	for range 2 {
		if err := Init(path); err != nil {
			t.Fatal(err)
		}
		for _, test := range tests {
			var got Episode
			if err := DB.Unscoped().First(&got, test.ep.ID).Error; err != nil || got.Status != test.want || got.ManualStatus != test.label || !got.UpdatedAt.Equal(test.ep.UpdatedAt) {
				t.Fatalf("迁移第 %g 集，得到 %q/%q，期望 %q/%q，错误 %v", test.ep.Number, got.Status, got.ManualStatus, test.want, test.label, err)
			}
		}
		var got Subscription
		if err := DB.First(&got, sub.ID).Error; err != nil || got.CurrentEpisodes != 2 || got.Completed {
			t.Fatalf("迁移进度 %+v %v", got, err)
		}
		conn, _ := DB.DB()
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
