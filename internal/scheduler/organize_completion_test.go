package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

type statusDownloader struct {
	core.Downloader
	task core.DownloadTask
}

func (m statusDownloader) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	return m.task, nil
}

func TestOrganizeRequiresConfirmedDownload(t *testing.T) {
	d := t.TempDir()
	originalDB := database.DB
	if err := database.Init(filepath.Join(d, "test.db")); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close(); database.DB = originalDB })
	from := filepath.Join(d, "downloading.mkv")
	if err := os.WriteFile(from, []byte("first chunk"), 0600); err != nil {
		t.Fatal(err)
	}
	s := database.Subscription{TitleCN: "Example", AnimeType: "tv"}
	if err := database.DB.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	ep := database.Episode{SubscriptionID: s.ID, Season: 1, Number: 1, TorrentHash: "synthetic", Status: "downloading"}
	if err := database.DB.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	task := core.DownloadTask{Hash: "synthetic", Status: "downloading", Progress: 0.25, ContentPath: from}
	sched := Scheduler{downloader: statusDownloader{task: task}, organizer: organizer.New("Episode{ep}.mkv", "", "", filepath.Join(d, "library"), "", false, nil)}
	sched.pollOrganizer(context.Background())
	if err := database.DB.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(from); err != nil || ep.Status != "downloading" || ep.FinalPath != "" {
		t.Fatalf("未完成任务已被整理: %+v, %v", ep, err)
	}
	if err := database.DB.Model(&ep).Update("status", "downloaded").Error; err != nil {
		t.Fatal(err)
	}
	sched.pollOrganizer(context.Background())
	if _, err := os.Stat(from); err != nil {
		t.Fatalf("数据库状态与下载器不一致时应保留源文件: %v", err)
	}
	task.Progress = 1
	task.Status = "seeding"
	sched.downloader = statusDownloader{task: task}
	sched.pollOrganizer(context.Background())
	if err := database.DB.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ep.FinalPath)
	if err != nil || string(data) != "first chunk" || ep.Status != "organized" {
		t.Fatalf("完成后未整理: %+v, %v", ep, err)
	}
}
