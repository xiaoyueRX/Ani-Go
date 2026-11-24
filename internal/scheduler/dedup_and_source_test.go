package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

func TestHexInfoHashDeduplicationIsCaseInsensitive(t *testing.T) {
	t.Setenv("DISK_MIN_FREE_GB", "0")
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", Season: 1, BangumiID: "1234"}
	savePipelineRow(t, db, &sub)

	upperHash := strings.Repeat("AB", 20)
	lowerHash := strings.ToLower(upperHash)
	// 历史记录可能仅存大写十六进制 TorrentHash 而缺少 OriginKey。
	savePipelineRow(t, db, &database.Episode{
		SubscriptionID: sub.ID,
		Season:         1,
		Number:         0,
		Title:          "Series Batch",
		OriginalName:   "Series Batch",
		Status:         "organized",
		TorrentHash:    upperHash,
		FinalPath:      "/lib/Series.mkv",
	})

	if !isEpisodeExists(lowerHash) {
		t.Fatalf("isEpisodeExists(%q) = false，期望命中大小写不同的十六进制哈希", lowerHash)
	}

	dl := &pipelineDownloader{}
	s := Scheduler{downloader: dl}
	added, err := s.enqueue(context.Background(), sub, core.TorrentItem{
		Title:    "Series Batch",
		URL:      "https://example.org/batch.torrent",
		InfoHash: lowerHash,
	}, 1, 0)
	if err != nil {
		t.Fatalf("enqueue 返回意外错误: %v", err)
	}
	if added || dl.calls.Load() != 0 {
		t.Fatalf("相同种子的大小写变体应被去重，实际 added=%v calls=%d", added, dl.calls.Load())
	}

	// 同理覆盖 64 位 BitTorrent v2 十六进制哈希。
	upperV2 := strings.Repeat("CD", 32)
	lowerV2 := strings.ToLower(upperV2)
	savePipelineRow(t, db, &database.Episode{
		SubscriptionID: sub.ID,
		Season:         1,
		Number:         0,
		Title:          "Series Batch v2",
		OriginalName:   "Series Batch v2",
		Status:         "organized",
		TorrentHash:    upperV2,
		FinalPath:      "/lib/Series-v2.mkv",
	})
	if !isEpisodeExists(lowerV2) {
		t.Fatalf("isEpisodeExists(%q) = false，期望命中 64 位十六进制哈希", lowerV2)
	}
}

func TestNonFileListerBoundSingleFileOrganizesWithoutJoiningFilenameTwice(t *testing.T) {
	t.Run("tv_bound_single_file", func(t *testing.T) {
		db := pipelineDB(t)
		sub := database.Subscription{TitleCN: "Show", AnimeType: "tv", Season: 1}
		savePipelineRow(t, db, &sub)

		dlDir := t.TempDir()
		libDir := t.TempDir()
		src := filepath.Join(dlDir, "Show.S01E01.mkv")
		if err := os.WriteFile(src, []byte("episode-payload"), 0600); err != nil {
			t.Fatal(err)
		}

		// FileSize=0 且 SourceModTime=0 令 verifiedBatchSource 为 false，强制走无 FileLister 分支。
		ep := database.Episode{
			SubscriptionID: sub.ID,
			Season:         1,
			Number:         1,
			Title:          "Show.S01E01",
			OriginalName:   "Show.S01E01",
			TorrentHash:    "singlehash01",
			Status:         "downloaded",
			DownloadRoot:   dlDir,
			SourceFile:     "Show.S01E01.mkv",
		}
		savePipelineRow(t, db, &ep)

		dl := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{
			tasks: []core.DownloadTask{{Hash: ep.TorrentHash, SavePath: dlDir, ContentPath: src, Progress: 1, Status: "completed"}},
		}}
		s := Scheduler{downloader: dl, organizer: organizer.New("{title_cn}/S{season:02}E{ep:02}{ext}", "", "", libDir, "", true, nil)}
		s.pollOrganizer(context.Background())

		if err := db.First(&ep, ep.ID).Error; err != nil {
			t.Fatal(err)
		}
		if ep.Status != "organized" || ep.FinalPath == "" {
			t.Fatalf("已绑定单文件剧集应成功整理入库，实际 status=%s reason=%q path=%q", ep.Status, ep.ReviewReason, ep.FinalPath)
		}
	})

	t.Run("confirmed_movie_single_file_and_drift_review", func(t *testing.T) {
		db := pipelineDB(t)
		sub := database.Subscription{TitleCN: "Film", AnimeType: "movie", TotalEpisodes: 1}
		savePipelineRow(t, db, &sub)

		dlDir := t.TempDir()
		libDir := t.TempDir()
		src := filepath.Join(dlDir, "Film.mkv")
		if err := os.WriteFile(src, []byte("film-payload"), 0600); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(src)
		if err != nil {
			t.Fatal(err)
		}

		ep := database.Episode{
			SubscriptionID: sub.ID,
			Season:         1,
			Number:         0,
			Title:          "Film",
			OriginalName:   "Film",
			TorrentHash:    "moviehash01",
			Status:         "downloaded",
			DownloadRoot:   dlDir,
			SourceFile:     "Film.mkv",
			FileSize:       info.Size(),
			SourceModTime:  info.ModTime().UnixNano(),
		}
		ep.ConfirmedIdentity = ep.ConfirmationSignature(database.ReviewMovieSelection)
		savePipelineRow(t, db, &ep)

		dl := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{
			tasks: []core.DownloadTask{{Hash: ep.TorrentHash, SavePath: dlDir, ContentPath: src, Progress: 1, Status: "completed"}},
		}}
		s := Scheduler{downloader: dl, organizer: organizer.New("", "Film/Movie{ext}", "", libDir, libDir, true, nil)}
		s.pollOrganizer(context.Background())

		if err := db.First(&ep, ep.ID).Error; err != nil {
			t.Fatal(err)
		}
		if ep.Status != "organized" || ep.FinalPath == "" {
			t.Fatalf("已确认电影单文件应整理入库，实际 status=%s reason=%q", ep.Status, ep.ReviewReason)
		}

		// 若源文件大小发生变化，非清单路径应转入 needs_review 而非拼接错误路径。
		drifted := database.Episode{
			SubscriptionID: sub.ID,
			Season:         1,
			Number:         0,
			Title:          "Film",
			OriginalName:   "Film",
			TorrentHash:    "moviehash02",
			Status:         "downloaded",
			DownloadRoot:   dlDir,
			SourceFile:     "Film.mkv",
			FileSize:       info.Size() + 100,
			SourceModTime:  info.ModTime().UnixNano(),
		}
		drifted.ConfirmedIdentity = drifted.ConfirmationSignature(database.ReviewMovieSelection)
		savePipelineRow(t, db, &drifted)

		dl.tasks = append(dl.tasks, core.DownloadTask{Hash: drifted.TorrentHash, SavePath: dlDir, ContentPath: src, Progress: 1, Status: "completed"})
		s.pollOrganizer(context.Background())

		if err := db.First(&drifted, drifted.ID).Error; err != nil {
			t.Fatal(err)
		}
		if drifted.Status != database.EpisodeNeedsReview || drifted.FinalPath != "" {
			t.Fatalf("确认指纹失效的电影应进入复核，实际 status=%s path=%q", drifted.Status, drifted.FinalPath)
		}
	})

	t.Run("mismatched_source_file_enters_review", func(t *testing.T) {
		db := pipelineDB(t)
		sub := database.Subscription{TitleCN: "Show", AnimeType: "tv", Season: 1}
		savePipelineRow(t, db, &sub)

		dlDir := t.TempDir()
		libDir := t.TempDir()
		src := filepath.Join(dlDir, "Show.S01E01.mkv")
		if err := os.WriteFile(src, []byte("episode-payload"), 0600); err != nil {
			t.Fatal(err)
		}

		ep := database.Episode{
			SubscriptionID: sub.ID,
			Season:         1,
			Number:         1,
			Title:          "Show.S01E01",
			OriginalName:   "Show.S01E01",
			TorrentHash:    "mismatch01",
			Status:         "downloaded",
			DownloadRoot:   dlDir,
			SourceFile:     "Other.mkv",
		}
		savePipelineRow(t, db, &ep)

		dl := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{
			tasks: []core.DownloadTask{{Hash: ep.TorrentHash, SavePath: dlDir, ContentPath: src, Progress: 1, Status: "completed"}},
		}}
		s := Scheduler{downloader: dl, organizer: organizer.New("{title_cn}/S{season:02}E{ep:02}{ext}", "", "", libDir, "", true, nil)}
		s.pollOrganizer(context.Background())

		if err := db.First(&ep, ep.ID).Error; err != nil {
			t.Fatal(err)
		}
		if ep.Status != database.EpisodeNeedsReview || ep.FinalPath != "" {
			t.Fatalf("单文件成员不匹配应进入 needs_review，实际 status=%s reason=%q path=%q", ep.Status, ep.ReviewReason, ep.FinalPath)
		}
	})
}
