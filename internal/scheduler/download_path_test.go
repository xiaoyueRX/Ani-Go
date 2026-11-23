package scheduler

import (
	"context"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

type captureDownloadPath struct {
	mockSchedDownloader
	paths []string
}

func (d *captureDownloadPath) Add(_ context.Context, _ core.TorrentItem, path string) error {
	d.paths = append(d.paths, path)
	return nil
}

func TestSubscriptionUsesDownloaderDirectoryAndKeepsLibraryDiskGuard(t *testing.T) {
	for _, tc := range []struct {
		name    string
		custom  string
		lowDisk bool
	}{
		{"default", "", false}, {"custom", "/client/downloads/series", false}, {"library-low", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := pipelineDB(t)
			t.Setenv("TV_BASE_PATH", t.TempDir())
			t.Setenv("DISK_MIN_FREE_GB", "0")
			if tc.lowDisk {
				t.Setenv("DISK_MIN_FREE_GB", "10000000000")
			}
			sub := database.Subscription{TitleCN: "Series", CustomPath: tc.custom}
			savePipelineRow(t, db, &sub)
			dl := new(captureDownloadPath)
			s := Scheduler{downloader: dl}
			ok, err := s.enqueue(t.Context(), sub, core.TorrentItem{InfoHash: "testhash", Title: "Series - 01", URL: "https://example.org/t"}, 1, 1)
			var ep database.Episode
			if e := db.First(&ep).Error; e != nil {
				t.Fatal(e)
			}
			if tc.lowDisk {
				if err == nil || ok || len(dl.paths) != 0 || ep.Status != "retrying" {
					t.Fatalf("low space submitted=%v paths=%v status=%s error=%v", ok, dl.paths, ep.Status, err)
				}
				t.Setenv("DISK_MIN_FREE_GB", "0")
				if err := s.retryEpisode(t.Context(), ep); err != nil {
					t.Fatal(err)
				}
			} else if !ok || err != nil {
				t.Fatalf("did not submit: %v", err)
			}
			if len(dl.paths) != 1 || dl.paths[0] != tc.custom {
				t.Fatalf("downloader received %q want %q", dl.paths, tc.custom)
			}
		})
	}
}
