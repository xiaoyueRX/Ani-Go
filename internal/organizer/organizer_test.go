package organizer

import (
	"testing"
)

func TestRenderTemplate_TV(t *testing.T) {
	tmpl := "{title_cn} ({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}"
	v := VarValues{
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
	v := VarValues{
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
	v := VarValues{Ep: 1}
	result := renderTemplate(tmpl, v)
	if result != "E01" {
		t.Errorf("补零结果 = %q, 期望 %q", result, "E01")
	}
}

func TestRenderTemplate_DoubleDigitNoExtraPad(t *testing.T) {
	tmpl := "E{ep:02}"
	v := VarValues{Ep: 12}
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
