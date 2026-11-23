package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

func TestOrganizerGroupsOnlySharedVolumeFailuresWithinEachPoll(t *testing.T) {
	db := pipelineDB(t)
	tv, ova, sources := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("TV_VOLUME_ID", "tv-marker")
	t.Setenv("OVA_VOLUME_ID", "ova-marker")
	sub := database.Subscription{TitleCN: "Series", AnimeType: "tv", Season: 1}
	special := database.Subscription{TitleCN: "Other", AnimeType: "ova", Season: 1}
	savePipelineRow(t, db, &sub)
	savePipelineRow(t, db, &special)
	for i, owner := range []database.Subscription{sub, sub, special, special} {
		name := fmt.Sprintf("%s S01E%02d.mkv", owner.TitleCN, i+1)
		path := filepath.Join(sources, name)
		if err := os.WriteFile(path, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		stat, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		ep := database.Episode{Title: name, OriginalName: name, SourceFile: name, DownloadRoot: sources, SourceModTime: stat.ModTime().UnixNano(), FileSize: stat.Size(), TorrentHash: fmt.Sprintf("%040x", i+1), SubscriptionID: owner.ID, Season: 1, Number: float32(i + 1), Status: "downloaded"}
		savePipelineRow(t, db, &ep)
	}
	bus := &capturedEvents{}
	s := Scheduler{organizer: organizer.New("episodes/E{ep:02}{ext}", "", "blocked/E{ep:02}{ext}", tv, "", true, nil, ova), bus: bus}
	var logs bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(original) })
	round := func(success, failed int) string {
		t.Helper()
		logs.Reset()
		bus.events = nil
		s.pollOrganizer(context.Background())
		if len(bus.events) != 1 || bus.events[0].Type != core.EventFileOrganized || bus.events[0].Payload["success"] != success || bus.events[0].Payload["failed"] != failed {
			t.Fatalf("整理结果事件改变：%+v", bus.events)
		}
		return logs.String()
	}
	for i := 0; i < 2; i++ {
		text := round(0, 4)
		if strings.Count(text, filepath.Clean(tv)) != 1 || strings.Count(text, filepath.Clean(ova)) != 1 || strings.Count(text, "2 个文件") != 2 {
			t.Fatalf("每轮需分别汇总两个不可用的卷：%s", text)
		}
	}
	for dir, marker := range map[string]string{tv: "tv-marker", ova: "ova-marker"} {
		if err := os.WriteFile(filepath.Join(dir, ".anigo-volume-id"), []byte(marker), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// 正确的卷下遇到单独的目标目录错误，应保留该条剧集的诊断。
	if err := os.WriteFile(filepath.Join(ova, "blocked"), []byte("ordinary file"), 0600); err != nil {
		t.Fatal(err)
	}
	if text := round(2, 2); !strings.Contains(text, "文件整理失败 [Other S01E03.mkv]") || !strings.Contains(text, "文件整理失败 [Other S01E04.mkv]") {
		t.Fatalf("目标文件错误被整卷提示吞掉：%s", text)
	}
	if err := os.Remove(filepath.Join(ova, "blocked")); err != nil {
		t.Fatal(err)
	}
	round(2, 0)
	var n int64
	if err := db.Model(&database.Episode{}).Where("status='organized'").Count(&n).Error; err != nil || n != 4 {
		t.Fatalf("修复后的下一轮应全部入库：%d %v", n, err)
	}
}
