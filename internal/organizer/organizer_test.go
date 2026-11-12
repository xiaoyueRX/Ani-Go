package organizer

import (
	"context"
	"os"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestRenderTemplate_TV(t *testing.T) {
	tmpl := "{title_cn} ({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}"
	v := core.VarValues{
		TitleCN: "鬼灭之刃",
		TitleEN: "Kimetsu no Yaiba",
		Year:    2023,
		Season:  3,
		Ep:      5,
		Ext:     ".mkv",
	}

	result := renderTemplate(tmpl, v)
	expected := "鬼灭之刃 (2023)/Season 3/Kimetsu no Yaiba S03E05.mkv"
	if result != expected {
		t.Errorf("渲染结果:\n  得到: %s\n  期望: %s", result, expected)
	}
}

func TestRenderTemplate_Movie(t *testing.T) {
	tmpl := "{title_cn} ({year})/{title_en}{ext}"
	v := core.VarValues{
		TitleCN: "鬼灭之刃 无限列车篇",
		TitleEN: "Demon Slayer Mugen Train",
		Year:    2020,
		Season:  0,
		Ep:      0,
		Ext:     ".mp4",
	}

	result := renderTemplate(tmpl, v)
	expected := "鬼灭之刃 无限列车篇 (2020)/Demon Slayer Mugen Train.mp4"
	if result != expected {
		t.Errorf("渲染结果:\n  得到: %s\n  期望: %s", result, expected)
	}
}

func TestRenderTemplate_SingleDigitZeroPad(t *testing.T) {
	tmpl := "E{ep:02}"
	v := core.VarValues{Ep: 1}
	result := renderTemplate(tmpl, v)
	if result != "E01" {
		t.Errorf("补零结果 = %q, 期望 %q", result, "E01")
	}
}

func TestRenderTemplate_DoubleDigitNoExtraPad(t *testing.T) {
	tmpl := "E{ep:02}"
	v := core.VarValues{Ep: 12}
	result := renderTemplate(tmpl, v)
	if result != "E12" {
		t.Errorf("补零结果 = %q, 期望 %q", result, "E12")
	}
}

func TestSanitizePath_IllegalChars(t *testing.T) {
	result := sanitizePath("test<>:\"|?*file")
	if result != "testfile" {
		t.Errorf("清理结果 = %q, 期望 %q", result, "testfile")
	}
}

func TestRenderTemplate_ZeroYearOmitsYearSegment(t *testing.T) {
	tests := []struct {
		tmpl     string
		v        core.VarValues
		expected string
	}{
		{
			tmpl: "{title_cn}{year}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
			v: core.VarValues{
				TitleCN: "孤独摇滚",
				TitleEN: "Bocchi the Rock",
				Season:  1,
				Ep:      1,
				Ext:     ".mkv",
			},
			expected: "孤独摇滚/Season 1/Bocchi the Rock S01E01.mkv",
		},
		{
			tmpl: "{title_cn} ({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
			v: core.VarValues{
				TitleCN: "孤独摇滚",
				TitleEN: "Bocchi the Rock",
				Year:    0,
				Season:  1,
				Ep:      1,
				Ext:     ".mkv",
			},
			expected: "孤独摇滚/Season 1/Bocchi the Rock S01E01.mkv",
		},
		{
			tmpl: "{title_cn}({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
			v: core.VarValues{
				TitleCN: "孤独摇滚",
				TitleEN: "Bocchi the Rock",
				Year:    -1,
				Season:  1,
				Ep:      1,
				Ext:     ".mkv",
			},
			expected: "孤独摇滚/Season 1/Bocchi the Rock S01E01.mkv",
		},
	}

	for _, tt := range tests {
		if got := renderTemplate(tt.tmpl, tt.v); got != tt.expected {
			t.Fatalf("模板 %q 渲染结果:\n  得到: %q\n  期望: %q", tt.tmpl, got, tt.expected)
		}
	}
}

func TestSelectTemplate_MediaTypeTemplates(t *testing.T) {
	org := New("tv-template", "movie-template", "other-template", "/tv", "/movie", false, nil)

	tests := []struct {
		animeType string
		want      string
	}{
		{animeType: "Movie", want: "movie-template"},
		{animeType: "OVA", want: "other-template"},
		{animeType: "", want: "other-template"},
	}
	for _, tt := range tests {
		if got := org.selectTemplate(core.Anime{Type: tt.animeType}); got != tt.want {
			t.Errorf("类型 %s 模板 = %q, 期望 %q", tt.animeType, got, tt.want)
		}
	}
}



func TestRenderTemplate_SeriesVariable(t *testing.T) {
	tmpl := "{series}/{title_cn} ({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}"
	
	// 测试用例 1: 指定了 SeriesID
	v1 := core.VarValues{
		Series:   "鬼灭之刃",
		SeriesID: "鬼灭之刃",
		TitleCN:  "鬼灭之刃 锻刀村篇",
		TitleEN:  "Demon Slayer Swordsmith Village Arc",
		Year:     2023,
		Season:   3,
		Ep:       1,
		Ext:      ".mkv",
	}
	res1 := renderTemplate(tmpl, v1)
	expected1 := "鬼灭之刃/鬼灭之刃 锻刀村篇 (2023)/Season 3/Demon Slayer Swordsmith Village Arc S03E01.mkv"
	if res1 != expected1 {
		t.Fatalf("res1 期望 %q, 得到 %q", expected1, res1)
	}

	// 测试用例 2: 未显式指定 Series，应回退为 TitleCN
	v2 := core.VarValues{
		Series:   "葬送的芙莉莲",
		TitleCN:  "葬送的芙莉莲",
		TitleEN:  "Frieren",
		Year:     2023,
		Season:   1,
		Ep:       10,
		Ext:      ".mp4",
	}
	res2 := renderTemplate(tmpl, v2)
	expected2 := "葬送的芙莉莲/葬送的芙莉莲 (2023)/Season 1/Frieren S01E10.mp4"
	if res2 != expected2 {
		t.Fatalf("res2 期望 %q, 得到 %q", expected2, res2)
	}
}

func TestRenderTemplate_TMDBIDPrefixStripping(t *testing.T) {
	tmpl := "{title_cn}/Season {season}/{title_en} [tmdbid={tmdb_id}] S{season:02}E{ep:02}{ext}"
	v := core.VarValues{
		TitleCN: "鬼灭之刃",
		TitleEN: "Kimetsu",
		Season:  1,
		Ep:      1,
		Ext:     ".mkv",
		TMDBID:  "tv/85937",
	}
	result := renderTemplate(tmpl, v)
	expected := "鬼灭之刃/Season 1/Kimetsu [tmdbid=85937] S01E01.mkv"
	if result != expected {
		t.Errorf("tmdb_id 剥离前缀失败: 得到 %q, 期望 %q", result, expected)
	}
}

func TestOrganizer_SameFileAndConflictHandling(t *testing.T) {
	tmpDir := t.TempDir()
	tvBase := tmpDir + "/TV"
	movieBase := tmpDir + "/Movie"
	org := New(
		"{title_cn}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
		"{title_cn} ({year})/{title_en}{ext}",
		"",
		tvBase,
		movieBase,
		true, // 硬链接模式
		nil,
	)

	srcFile := tmpDir + "/download_01.mkv"
	initialContent := []byte("original video stream data")
	if err := os.WriteFile(srcFile, initialContent, 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	anime := core.Anime{
		TitleCN: "迷宫饭",
		TitleEN: "Dungeon Meshi",
		Type:    "TV",
	}
	ep := core.Episode{
		Season: 1,
		Number: 1,
	}

	ctx := context.Background()

	// 1. 首次正常整理建立硬链接
	path1, err := org.Organize(ctx, srcFile, anime, ep)
	if err != nil {
		t.Fatalf("首次整理失败: %v", err)
	}
	content1, _ := os.ReadFile(path1)
	if string(content1) != string(initialContent) {
		t.Fatalf("文件内容不匹配")
	}

	// 2. 第二次整理（SameFile 场景）：目标路径与源文件已指向同一物理硬链接
	path2, err := org.Organize(ctx, srcFile, anime, ep)
	if err != nil {
		t.Fatalf("同一文件重复整理失败: %v", err)
	}
	if path1 != path2 {
		t.Errorf("路径应一致: %s vs %s", path1, path2)
	}
	content2, _ := os.ReadFile(path2)
	if len(content2) == 0 {
		t.Fatalf("严重漏洞：文件已被截断为 0 字节！")
	}

	// 3. 冲突场景：目标路径已被另一个旧文件占据，整理时应安全清理并重建新链接
	conflictContent := []byte("old obsolete file version")
	_ = os.Remove(path2)
	_ = os.WriteFile(path2, conflictContent, 0644)

	path3, err := org.Organize(ctx, srcFile, anime, ep)
	if err != nil {
		t.Fatalf("覆盖旧冲突文件失败: %v", err)
	}
	content3, _ := os.ReadFile(path3)
	if string(content3) != string(initialContent) {
		t.Errorf("冲突覆盖后内容不匹配，得到: %s", string(content3))
	}
}
