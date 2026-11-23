package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

func TestMoveCanRetryAfterOrganizationCommitFails(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", AnimeType: "tv"}
	savePipelineRow(t, db, &sub)
	ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, TorrentHash: "hash", Status: "downloaded"}
	savePipelineRow(t, db, &ep)
	source := filepath.Join(t.TempDir(), "E01.mkv")
	if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	dl := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{tasks: []core.DownloadTask{{Hash: "hash", ContentPath: source, Progress: 1, Status: "completed"}}}}
	lib := t.TempDir()
	mk := func() *Scheduler {
		return &Scheduler{downloader: dl, organizer: organizer.New("series/E01{ext}", "", "", lib, "", false, nil)}
	}
	if err := db.Exec("CREATE TRIGGER reject_organization BEFORE UPDATE ON episodes WHEN NEW.status='organized' BEGIN SELECT RAISE(ABORT,'test'); END").Error; err != nil {
		t.Fatal(err)
	}
	mk().pollOrganizer(context.Background())
	if b, err := os.ReadFile(source); err != nil || string(b) != "payload" {
		t.Fatalf("database rejected organization but original disappeared: %v", err)
	}
	if err := db.Exec("DROP TRIGGER reject_organization").Error; err != nil {
		t.Fatal(err)
	}
	mk().pollOrganizer(context.Background())
	if err := db.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(ep.FinalPath); ep.Status != "organized" || err != nil || string(b) != "payload" {
		t.Fatalf("restart did not finish move: status %q file %q (%v)", ep.Status, string(b), err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("move left original: %v", err)
	}
}

func TestOrganizerUsesSelectedCompleteFilesForSingleAndBatch(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(fmt.Sprintf("batch=%v", batch), func(t *testing.T) {
			db := pipelineDB(t)
			sub := database.Subscription{TitleCN: "Show", AnimeType: "tv", Season: 1}
			savePipelineRow(t, db, &sub)
			ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, OriginalName: "Show - 01", Status: "downloaded", TorrentHash: "abcd"}
			if batch {
				ep.Number = 0
				ep.OriginalName = "Show [01-02]"
			}
			savePipelineRow(t, db, &ep)
			dir := t.TempDir()
			pack := filepath.Join(dir, "Show")
			if err := os.Mkdir(pack, 0700); err != nil {
				t.Fatal(err)
			}
			for n, content := range map[string]string{"E01.mkv": "one", "E02.mkv": "incomplete preallocated file", "E03.mkv": "unrelated large video accidentally in same directory"} {
				if err := os.WriteFile(filepath.Join(pack, n), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			serve := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/auth/login":
					fmt.Fprint(w, "Ok.")
				case "/api/v2/torrents/info":
					json.NewEncoder(w).Encode([]map[string]any{{"hash": "abcd", "name": "Show", "content_path": pack, "save_path": dir, "state": "uploading", "size": 3, "amount_left": 0, "progress": 1}})
				case "/api/v2/torrents/files":
					fmt.Fprint(w, `[{"name":"Show/E01.mkv","priority":1,"progress":1,"size":3},{"name":"Show/E02.mkv","priority":0,"progress":0.1,"size":28}]`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer serve.Close()
			s := Scheduler{downloader: downloader.NewDynamicDownloader(downloader.NewQBittorrent(serve.URL, "", "", "")), organizer: organizer.New("episodes/E{ep:02}{ext}", "", "", t.TempDir(), "", true, nil)}
			s.pollOrganizer(context.Background())
			var rows []database.Episode
			if err := db.Where("status = ?", "organized").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].Number != 1 {
				t.Fatalf("wrong episodes archived: %+v", rows)
			}
			if content, err := os.ReadFile(rows[0].FinalPath); err != nil || string(content) != "one" {
				t.Fatalf("wrong file archived: %q (%v)", content, err)
			}
		})
	}
}

func TestMoveRecoveryAcrossMountsAndLegacyRename(t *testing.T) {
	for _, mode := range []string{"hardlink", "move", "renaming"} {
		t.Run(mode, func(t *testing.T) {
			db := pipelineDB(t)
			sub := database.Subscription{TitleCN: "Series", AnimeType: "tv"}
			savePipelineRow(t, db, &sub)
			src := filepath.Join(t.TempDir(), "old.mkv")
			if err := os.WriteFile(src, []byte("original seed content"), 0600); err != nil {
				t.Fatal(err)
			}
			lib, err := os.MkdirTemp("/dev/shm", "anigo-cross-volume-")
			if err != nil {
				t.Skip("second filesystem not available")
			}
			defer os.RemoveAll(lib)
			probe := filepath.Join(lib, "probe")
			if err := os.Link(src, probe); err == nil {
				t.Skip("test needs different filesystems")
			}
			ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, Status: "downloaded", TorrentHash: "hash"}
			if mode == "renaming" {
				ep.Status = "renaming"
				ep.FinalPath = src
			}
			savePipelineRow(t, db, &ep)
			dl := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{tasks: []core.DownloadTask{{Hash: "hash", ContentPath: src, Progress: 1, Status: "completed"}}}}
			newService := func() *Scheduler {
				return &Scheduler{downloader: dl, organizer: organizer.New("series/E01{ext}", "", "", lib, "", mode == "hardlink", nil)}
			}
			if err := db.Exec("CREATE TRIGGER reject_organization BEFORE UPDATE ON episodes WHEN NEW.status='organized' BEGIN SELECT RAISE(ABORT,'test'); END").Error; err != nil {
				t.Fatal(err)
			}
			newService().pollOrganizer(context.Background())
			if b, err := os.ReadFile(src); err != nil || string(b) != "original seed content" {
				t.Fatalf("original lost: %q %v", b, err)
			}
			if err := db.Exec("DROP TRIGGER reject_organization").Error; err != nil {
				t.Fatal(err)
			}
			newService().pollOrganizer(context.Background())
			if err := db.First(&ep, ep.ID).Error; err != nil {
				t.Fatal(err)
			}
			if ep.Status != "organized" {
				t.Fatalf("restart did not finish %s: %s", mode, ep.Status)
			}
			if b, err := os.ReadFile(ep.FinalPath); err != nil || string(b) != "original seed content" {
				t.Fatal("invalid output")
			}
			_, err = os.Stat(src)
			if mode == "hardlink" && err != nil || mode != "hardlink" && !os.IsNotExist(err) {
				t.Fatalf("%s source status %v", mode, err)
			}
		})
	}
}

func TestRestartFinishesOnlyUnchangedPendingMoveSource(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series"}
	savePipelineRow(t, db, &sub)
	for i, changed := range []bool{false, true} {
		dir := t.TempDir()
		src := filepath.Join(dir, "src.mkv")
		dst := filepath.Join(dir, "dst.mkv")
		for _, name := range []string{src, dst} {
			if err := os.WriteFile(name, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		info, err := os.Stat(src)
		if err != nil {
			t.Fatal(err)
		}
		ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: float32(i + 1), Status: "organized", FinalPath: dst, MoveSourcePath: src, MoveSourceModTime: info.ModTime().UnixNano()}
		savePipelineRow(t, db, &ep)
		if changed {
			if err := os.WriteFile(src, []byte("replaced"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(src, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
		}
		s := &Scheduler{}
		s.pollOrganizer(context.Background())
		if err := db.First(&ep, ep.ID).Error; err != nil {
			t.Fatal(err)
		}
		_, err = os.Stat(src)
		if changed && (err != nil || ep.MoveSourcePath == "") || !changed && (!os.IsNotExist(err) || ep.MoveSourcePath != "") {
			t.Fatalf("cleaned wrong pending source: changed=%v err=%v pending=%q", changed, err, ep.MoveSourcePath)
		}
	}
}

func TestGuardedPendingMoveKeepsSourceUntilOriginalVolumeReturns(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Show", AnimeType: "TV"}
	savePipelineRow(t, db, &sub)
	lib := t.TempDir()
	marker := filepath.Join(lib, ".anigo-volume-id")
	source := filepath.Join(t.TempDir(), "download.mkv")
	write := func(path, data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TV_VOLUME_ID", "correct")
	write(marker, "correct\n")
	write(source, "fixture")
	o := organizer.New("Series/E01.mkv", "", "", lib, "", false, nil)
	s := &Scheduler{organizer: o}
	ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, Status: "downloaded"}
	savePipelineRow(t, db, &ep)
	if _, err := s.commitOrganization(t.Context(), ep, source, core.Anime{Type: "TV"}, core.Episode{Season: 1, Number: 1}); err != nil {
		t.Fatal(err)
	}
	// 模拟重新启动时库目录已经换盘；替代盘碰巧含相同字节也不能据此删除源。
	write(marker, "different")
	t.Setenv("TV_VOLUME_ID", "")
	restarted := &Scheduler{organizer: organizer.New("", "", "", t.TempDir(), "", false, nil)}
	restarted.pollOrganizer(t.Context())
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("removed pending source after marker changed: %v", err)
	}
	if err := db.First(&ep, ep.ID).Error; err != nil || ep.MoveSourcePath != source {
		t.Fatalf("pending move lost: %v %q", err, ep.MoveSourcePath)
	}
	write(marker, "correct")
	restarted.pollOrganizer(t.Context())
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("restored volume cleanup = %v", err)
	}
}

func TestPreGuardPendingMoveChecksConfiguredLibrary(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Legacy", AnimeType: "tv"}
	savePipelineRow(t, db, &sub)
	t.Setenv("TV_VOLUME_ID", "protected")
	lib := t.TempDir()
	outside := t.TempDir()
	s := &Scheduler{organizer: organizer.New("", "", "", lib, outside, false, nil, t.TempDir())}
	var originals []string
	for i, root := range []string{lib, outside} {
		dst := filepath.Join(root, "ep.mkv")
		src := filepath.Join(t.TempDir(), "ep.mkv")
		originals = append(originals, src)
		for _, path := range []string{dst, src} {
			if err := os.WriteFile(path, []byte("same"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		stat, err := os.Stat(src)
		if err != nil {
			t.Fatal(err)
		}
		ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: float32(i + 1), Status: "organized", FinalPath: dst, MoveSourcePath: src, MoveSourceModTime: stat.ModTime().UnixNano()}
		savePipelineRow(t, db, &ep)
	}
	s.pollOrganizer(t.Context())
	for _, src := range originals {
		if _, err := os.Stat(src); err != nil {
			t.Fatalf("source lost while volume unknown: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(lib, ".anigo-volume-id"), []byte("protected"), 0600); err != nil {
		t.Fatal(err)
	}
	s.pollOrganizer(t.Context())
	if _, err := os.Stat(originals[0]); !os.IsNotExist(err) {
		t.Fatalf("cleanup on returning volume = %v", err)
	}
	if _, err := os.Stat(originals[1]); err != nil {
		t.Fatalf("removed source with unknown library: %v", err)
	}
}
