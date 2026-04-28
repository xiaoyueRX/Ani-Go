package source

import (
	"testing"
)

func TestParseMikanTitle_ChineseBracket(t *testing.T) {
	// 【字幕组】番剧名 - 01
	info := ParseMikanTitle("【千夏字幕组】鬼灭之刃 游郭篇 - 01")
	if info.Subgroup != "千夏字幕组" {
		t.Errorf("字幕组 = %q, 期望 %q", info.Subgroup, "千夏字幕组")
	}
	if info.Episode != 1 {
		t.Errorf("集数 = %g, 期望 1", info.Episode)
	}
}

func TestParseMikanTitle_JapaneseStyle(t *testing.T) {
	// [字幕組] 番剧名 第01話
	info := ParseMikanTitle("[喵萌奶茶屋] 迷宫饭 第07話")
	if info.Subgroup != "喵萌奶茶屋" {
		t.Errorf("字幕组 = %q, 期望 %q", info.Subgroup, "喵萌奶茶屋")
	}
	if info.Episode != 7 {
		t.Errorf("集数 = %g, 期望 7", info.Episode)
	}
}

func TestParseMikanTitle_SeasonEpisode(t *testing.T) {
	// 含 SxxExx 格式
	info := ParseMikanTitle("[桜都字幕组] 无职转生 S02E03 1080p")
	if info.Subgroup != "桜都字幕组" {
		t.Errorf("字幕组 = %q, 期望 %q", info.Subgroup, "桜都字幕组")
	}
	if info.Season != 2 {
		t.Errorf("季数 = %d, 期望 2", info.Season)
	}
	if info.Episode != 3 {
		t.Errorf("集数 = %g, 期望 3", info.Episode)
	}
}

func TestParseMikanTitle_Resolution(t *testing.T) {
	info := ParseMikanTitle("【VCB-Studio】攻壳机动队 SAC_2045 - 01 1080p")
	if info.Resolution != "1080p" {
		t.Errorf("分辨率 = %q, 期望 %q", info.Resolution, "1080p")
	}
}

func TestParseMikanTitle_SeasonTitle(t *testing.T) {
	info := ParseMikanTitle("[LoliHouse] 鬼灭之刃 第二季 - 01")
	if info.Season != 2 {
		t.Errorf("季数 = %d, 期望 2", info.Season)
	}
}

func TestParseMikanTitle_NoSubgroup(t *testing.T) {
	// 无字幕组标记
	info := ParseMikanTitle("鬼灭之刃 游郭篇 - 12")
	if info.Subgroup != "" {
		t.Errorf("字幕组 = %q, 期望空", info.Subgroup)
	}
	if info.Episode != 12 {
		t.Errorf("集数 = %g, 期望 12", info.Episode)
	}
}

func TestParseMikanTitle_HashStyle(t *testing.T) {
	// #01 样式
	info := ParseMikanTitle("[KRL] 假面骑士极狐 #45")
	if info.Episode != 45 {
		t.Errorf("集数 = %g, 期望 45", info.Episode)
	}
}

func TestParseMikanTitle_EPStyle(t *testing.T) {
	// EP01 样式
	info := ParseMikanTitle("【MingY】葬送的芙莉莲 EP28")
	if info.Subgroup != "MingY" {
		t.Errorf("字幕组 = %q, 期望 %q", info.Subgroup, "MingY")
	}
	if info.Episode != 28 {
		t.Errorf("集数 = %g, 期望 28", info.Episode)
	}
}

func TestParseMikanTitle_PointFiveEpisode(t *testing.T) {
	// EPStyle with .5
	info := ParseMikanTitle("【MingY】葬送的芙莉莲 EP12.5")
	if info.Episode != 12.5 {
		t.Errorf("集数 = %g, 期望 12.5", info.Episode)
	}
}
