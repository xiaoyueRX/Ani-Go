package organizer

import (
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
