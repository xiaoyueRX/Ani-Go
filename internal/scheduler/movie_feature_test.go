package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

func TestMovieFeatureSelectionUsesFullManifestAndReleaseName(t *testing.T) {
	for _, tc := range []struct {
		title, release, file   string
		partial, bound, review bool
	}{
		{"Film", "Film", "Film.mkv", false, false, false},
		{"Film", "Film", "Film - Trailer.mkv", false, false, true},
		{"Film", "Film", "PV.mkv", false, false, true},
		{"Film", "Film [预告片]", "video.mkv", false, false, true},
		{"Film", "Film", "video.mkv", true, false, true},
		{"Film", "Film", "selected.mkv", true, true, true},
		{"Film", "Film", "Trailer.mkv", false, true, true},
		{"Trailer Park Boys", "[Group] Trailer.Park.Boys", "Trailer Park Boys.mkv", false, false, false},
		{"The Menu", "The Menu", "The Menu.mkv", false, false, false},
	} {
		t.Run(tc.release+"/"+tc.file, func(t *testing.T) {
			db := pipelineDB(t)
			sub := database.Subscription{TitleCN: tc.title, AnimeType: "Movie", TotalEpisodes: 1}
			savePipelineRow(t, db, &sub)
			root, lib := t.TempDir(), t.TempDir()
			path := filepath.Join(root, tc.file)
			if err := os.WriteFile(path, []byte("film"), 0600); err != nil {
				t.Fatal(err)
			}
			ep := database.Episode{SubscriptionID: sub.ID, TorrentHash: "movie", Status: "downloaded", Title: tc.release, OriginalName: tc.release}
			if tc.bound {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				ep.DownloadRoot, ep.SourceFile, ep.FileSize, ep.SourceModTime = root, tc.file, info.Size(), info.ModTime().UnixNano()
			}
			savePipelineRow(t, db, &ep)
			dl := &memberListDownloader{files: downloader.FileSelection{Partial: tc.partial, Files: []downloader.CompletedFile{{Path: path, Size: 4}}}}
			dl.tasks = []core.DownloadTask{{Hash: ep.TorrentHash, SavePath: root, ContentPath: path, Progress: 1, Status: "completed"}}
			s := Scheduler{downloader: dl, organizer: organizer.New("Show/TV{ext}", "Film/Movie{ext}", "", lib, lib, tc.title != "The Menu", nil)}
			s.pollOrganizer(t.Context())
			if err := db.First(&ep, ep.ID).Error; err != nil {
				t.Fatal(err)
			}
			if tc.review {
				if ep.Status != database.EpisodeNeedsReview || ep.ReviewKind != database.ReviewMovieSelection || ep.FinalPath != "" || (ep.MovieFeature != nil && *ep.MovieFeature) {
					t.Fatalf("应保留来源等待确认，得到 status=%s, kind=%s, path=%s", ep.Status, ep.ReviewKind, ep.FinalPath)
				}
			} else if ep.Status != "organized" || ep.FinalPath == "" || ep.Number != 0 || ep.MovieFeature == nil || !*ep.MovieFeature {
				t.Fatalf("未按正常电影源入库：%+v", ep)
			}
		})
	}
}

func TestCollectedMovieContinuesToAcceptNewRSSRelease(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Harbor Movie", AnimeType: "Movie", TotalEpisodes: 1, RSSURL: "https://example.test/movie"}
	savePipelineRow(t, db, &sub)
	feature := true
	organized := time.Now()
	first := database.Episode{SubscriptionID: sub.ID, Number: 0, MovieFeature: &feature, OrganizedAt: &organized, Status: "organized", FinalPath: "/films/Harbor Movie.mkv", TorrentHash: "first"}
	savePipelineRow(t, db, &first)
	if current, err := database.SyncEpisodeProgress(db, sub.ID, true); err != nil || current != 1 {
		t.Fatalf("正片未完成订阅：%d %v", current, err)
	}
	if err := db.First(&sub, sub.ID).Error; err != nil || !sub.Completed {
		t.Fatalf("订阅未标记完成：%v %v", sub.Completed, err)
	}
	feed := &subscriptionFeedFixture{items: []core.TorrentItem{{Title: "[Group] Harbor Movie [Movie]", URL: "https://example.test/new.torrent", InfoHash: "second"}}}
	dl := &feedDownloader{}
	sched := Scheduler{cfg: &config.Config{}, source: feed, downloader: dl}
	sched.pollRSS(t.Context())
	sched.pollRSS(t.Context())
	var next database.Episode
	if err := db.Where("origin_key = ?", "second").First(&next).Error; err != nil || len(dl.items) != 1 || next.SubscriptionID != sub.ID || next.Number != 0 {
		t.Fatalf("后续 RSS 版本未单独入队：%+v 投递数=%d %v", next, len(dl.items), err)
	}
}
