package scheduler

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

// 仅在测试子进程中暂停，让父进程能在真实 SQLite 提交的前后分别硬终止它。
type interruptibleOrganizer struct {
	*organizer.TVOrganizer
	dir, phase string
}

func (o interruptibleOrganizer) OrganizeVersionAndCommit(ctx context.Context, path string, anime core.Anime, ep core.Episode, key string, commit func(string, *organizer.MoveSource) error) (string, error) {
	return o.TVOrganizer.OrganizeVersionAndCommit(ctx, path, anime, ep, key, func(dst string, move *organizer.MoveSource) error {
		if o.phase == "after" {
			if err := commit(dst, move); err != nil {
				return err
			}
		}
		if err := os.WriteFile(filepath.Join(o.dir, "checkpoint"), []byte(o.phase), 0600); err != nil {
			return err
		}
		for {
			time.Sleep(time.Second)
		}
	})
}

func TestKilledOrganizerResumesFromSQLite(t *testing.T) {
	const envDir = "ANIGO_TEST_KILLED_ORGANIZER_DIR"
	newScheduler := func(dir string, org core.Organizer) *Scheduler {
		dl := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{tasks: []core.DownloadTask{{Hash: "hash", ContentPath: filepath.Join(dir, "source.mkv"), Status: "completed", Progress: 1}}}}
		return &Scheduler{organizer: org, downloader: dl}
	}
	makeOrg := func(dir string) *organizer.TVOrganizer {
		return organizer.New("Show/E01{ext}", "", "", filepath.Join(dir, "library"), "", false, nil)
	}
	if dir := os.Getenv(envDir); dir != "" {
		if err := database.Init(filepath.Join(dir, "state.db")); err != nil {
			t.Fatal(err)
		}
		sub := database.Subscription{TitleCN: "Show", AnimeType: "tv"}
		savePipelineRow(t, database.DB, &sub)
		ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, Status: "downloaded", TorrentHash: "hash"}
		savePipelineRow(t, database.DB, &ep)
		if err := os.WriteFile(filepath.Join(dir, "source.mkv"), []byte("complete episode"), 0600); err != nil {
			t.Fatal(err)
		}
		newScheduler(dir, interruptibleOrganizer{makeOrg(dir), dir, os.Getenv("ANIGO_TEST_KILLED_ORGANIZER_PHASE")}).pollOrganizer(context.Background())
		t.Fatal("未到达模拟宕机检查点")
	}
	for _, phase := range []string{"before", "after"} {
		t.Run(phase, func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestKilledOrganizerResumesFromSQLite$")
			child.Env = append(os.Environ(), envDir+"="+dir, "ANIGO_TEST_KILLED_ORGANIZER_PHASE="+phase)
			var logs bytes.Buffer
			child.Stdout = &logs
			child.Stderr = &logs
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			for ctx.Err() == nil {
				if b, err := os.ReadFile(filepath.Join(dir, "checkpoint")); err == nil && string(b) == phase {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			_ = child.Process.Kill()
			_ = child.Wait()
			if ctx.Err() != nil {
				t.Fatalf("子进程未就绪 %v: %s", ctx.Err(), logs.String())
			}
			old := database.DB
			if err := database.Init(filepath.Join(dir, "state.db")); err != nil {
				t.Fatal(err)
			}
			conn, err := database.DB.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close(); database.DB = old })
			var before database.Episode
			if err := database.DB.First(&before).Error; err != nil {
				t.Fatal(err)
			}
			if phase == "before" && before.Status != "downloaded" || phase == "after" && (before.Status != "organized" || before.MoveSourcePath == "") {
				t.Fatalf("错误提交检查点 %+v", before)
			}
			if _, err := os.Stat(filepath.Join(dir, "source.mkv")); err != nil {
				t.Fatal(err)
			}
			newScheduler(dir, makeOrg(dir)).pollOrganizer(context.Background())
			var after database.Episode
			if err := database.DB.First(&after).Error; err != nil {
				t.Fatal(err)
			}
			if content, err := os.ReadFile(after.FinalPath); err != nil || string(content) != "complete episode" || after.Status != "organized" || after.MoveSourcePath != "" {
				t.Fatalf("恢复失败 %+v (%v)", after, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "source.mkv")); !os.IsNotExist(err) {
				t.Fatalf("源文件清理状态 %v", err)
			}
		})
	}
}
