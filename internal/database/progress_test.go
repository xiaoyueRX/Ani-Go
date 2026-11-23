package database

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSeasonProgressDoesNotReplaceMissingEpisodesWithOutOfRangeNumbers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "progress.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&Subscription{}, &Episode{}); err != nil {
		t.Fatal(err)
	}
	sub := Subscription{TitleCN: "测试", Season: 2, TotalEpisodes: 2}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	rows := []Episode{
		{SubscriptionID: sub.ID, Season: 2, Number: 1, Status: "organized"},
		{SubscriptionID: sub.ID, Season: 2, Number: 1, Status: "organized"},
		{SubscriptionID: sub.ID, Season: 2, Number: 2, Status: "needs_review", ManualStatus: "completed"},
		{SubscriptionID: sub.ID, Season: 2, Number: 13, Status: "organized"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	check := func(want int, complete bool) {
		t.Helper()
		counts, err := FinishedEpisodeCounts(db, []uint{sub.ID})
		if err != nil || counts[sub.ID] != want {
			t.Errorf("订阅列表进度 %v，期望 %d：%v", counts, want, err)
		}
		count, err := SyncEpisodeProgress(db, sub.ID, true)
		if err != nil {
			t.Fatal(err)
		}
		var saved Subscription
		if err := db.First(&saved, sub.ID).Error; err != nil {
			t.Fatal(err)
		}
		if count != want || saved.CurrentEpisodes != want || saved.Completed != complete {
			t.Errorf("已整理 %d，落库 %d，完结 %t；期望 %d、%t", count, saved.CurrentEpisodes, saved.Completed, want, complete)
		}
	}
	check(1, false)
	if err := db.Model(&rows[2]).Update("status", "organized").Error; err != nil {
		t.Fatal(err)
	}
	check(2, true)
	if err := db.Model(&sub).UpdateColumn("total_episodes", 0).Error; err != nil {
		t.Fatal(err)
	}
	check(3, true)
}
