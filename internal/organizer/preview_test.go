package organizer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestTVOrganizer_Preview(t *testing.T) {
	tmpDir := t.TempDir()
	org := New(
		"{title_cn}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
		"{title_cn} ({year})/{title_en}{ext}",
		"{title_cn}/Specials/{title_en} S00E{ep:02}{ext}",
		filepath.Join(tmpDir, "TV"),
		filepath.Join(tmpDir, "Movies"),
		true,
		nil,
		filepath.Join(tmpDir, "OVA"),
	)

	anime := core.Anime{
		ID:      "1",
		TitleCN: "葬送的芙莉莲",
		TitleEN: "Sousou no Frieren",
		Type:    "TV",
		Year:    2023,
	}
	ep := core.Episode{
		Season: 1,
		Number: 8,
	}

	res, err := org.Preview(context.Background(), "/downloads/frieren_08.mkv", anime, ep)
	if err != nil {
		t.Fatalf("Preview 发生错误: %v", err)
	}

	if res.OriginalPath != "/downloads/frieren_08.mkv" {
		t.Errorf("OriginalPath 期望 /downloads/frieren_08.mkv, 实际 %s", res.OriginalPath)
	}

	expectedRendered := "葬送的芙莉莲/Season 1/Sousou no Frieren S01E08"
	if res.RenderedPath != expectedRendered {
		t.Errorf("RenderedPath 期望 %s, 实际 %s", expectedRendered, res.RenderedPath)
	}

	expectedFinal := filepath.Join(tmpDir, "TV", "葬送的芙莉莲", "Season 1", "Sousou no Frieren S01E08.mkv")
	if res.FinalPath != expectedFinal {
		t.Errorf("FinalPath 期望 %s, 实际 %s", expectedFinal, res.FinalPath)
	}

	if res.Action != "hardlink" {
		t.Errorf("Action 期望 hardlink, 实际 %s", res.Action)
	}

	if res.Exists {
		t.Errorf("Exists 期望 false, 实际 true")
	}
}

func TestTVOrganizer_PreviewBatch(t *testing.T) {
	tmpDir := t.TempDir()
	org := New(
		"{title_cn}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
		"{title_cn} ({year})/{title_en}{ext}",
		"{title_cn}/Specials/{title_en} S00E{ep:02}{ext}",
		filepath.Join(tmpDir, "TV"),
		filepath.Join(tmpDir, "Movies"),
		false,
		nil,
	)

	items := []PreviewItem{
		{
			FilePath: "/dl/ep1.mp4",
			Anime: core.Anime{
				TitleCN: "迷宫饭",
				TitleEN: "Dungeon Meshi",
				Type:    "TV",
			},
			Episode: core.Episode{
				Season: 1,
				Number: 1,
			},
		},
		{
			FilePath: "/dl/ep2.mp4",
			Anime: core.Anime{
				TitleCN: "迷宫饭",
				TitleEN: "Dungeon Meshi",
				Type:    "TV",
			},
			Episode: core.Episode{
				Season: 1,
				Number: 2,
			},
		},
	}

	results, err := org.PreviewBatch(context.Background(), items)
	if err != nil {
		t.Fatalf("PreviewBatch 失败: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("结果数量期望 2, 实际 %d", len(results))
	}

	if results[0].Action != "move" {
		t.Errorf("Action 期望 move, 实际 %s", results[0].Action)
	}

	expected1 := filepath.Join(tmpDir, "TV", "迷宫饭", "Season 1", "Dungeon Meshi S01E01.mp4")
	if results[0].FinalPath != expected1 {
		t.Errorf("Item 0 FinalPath 期望 %s, 实际 %s", expected1, results[0].FinalPath)
	}

	expected2 := filepath.Join(tmpDir, "TV", "迷宫饭", "Season 1", "Dungeon Meshi S01E02.mp4")
	if results[1].FinalPath != expected2 {
		t.Errorf("Item 1 FinalPath 期望 %s, 实际 %s", expected2, results[1].FinalPath)
	}
}

func TestTVOrganizer_Preview_HookCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	org := New(
		"{title_cn}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
		"", "",
		tmpDir, tmpDir,
		true,
		nil,
	)

	org.GetHookManager().RegisterNamingHook(core.PriorityHigh, "cancel-hook", func(ctx context.Context, input interface{}) (interface{}, error) {
		return core.NamingHookOutput{
			Cancel: true,
			Reason: "测试拦截取消",
		}, nil
	})

	anime := core.Anime{TitleCN: "测试番剧", Type: "TV"}
	ep := core.Episode{Season: 1, Number: 1}

	res, err := org.Preview(context.Background(), "test.mkv", anime, ep)
	if err == nil {
		t.Fatalf("期望返回错误，但 err 为 nil")
	}

	if res.Action != "cancelled" {
		t.Errorf("Action 期望 cancelled, 实际 %s", res.Action)
	}
	if res.Error != "测试拦截取消" {
		t.Errorf("Error 期望 '测试拦截取消', 实际 '%s'", res.Error)
	}
}
