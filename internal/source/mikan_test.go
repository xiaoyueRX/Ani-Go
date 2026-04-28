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
	info := ParseMikanTitle("【MingY】葬送的芙莉莲 EP12.5")
	if info.Episode != 12.5 {
		t.Errorf("集数 = %g, 期望 12.5", info.Episode)
	}
}

func TestParseMikanTitle_VolPattern(t *testing.T) {
	info := ParseMikanTitle("[DMG] 某科学的超电磁炮 Vol 5")
	if info.Episode != 5 {
		t.Errorf("集数 = %g, 期望 5", info.Episode)
	}
}

func TestParseMikanTitle_ChineseBracketEp(t *testing.T) {
	// 【01】中文方括号格式
	info := ParseMikanTitle("【极影字幕社】某科学的超电磁炮T【01】")
	if info.Episode != 1 {
		t.Errorf("集数 = %g, 期望 1", info.Episode)
	}
	if info.Subgroup != "极影字幕社" {
		t.Errorf("字幕组 = %q, 期望 %q", info.Subgroup, "极影字幕社")
	}
}

func TestParseMikanTitle_SquareBracketEp(t *testing.T) {
	// [01] 英文方括号格式
	info := ParseMikanTitle("Fate/Zero [01] 1080p")
	if info.Episode != 1 {
		t.Errorf("集数 = %g, 期望 1", info.Episode)
	}
}

func TestParseMikanTitle_SquareBracketWithVersion(t *testing.T) {
	// [01v2] 带版本号
	info := ParseMikanTitle("[DMG] 某动画 [01v2]")
	if info.Episode != 1 {
		t.Errorf("集数 = %g, 期望 1", info.Episode)
	}
	if info.Version != 2 {
		t.Errorf("版本号 = %d, 期望 2", info.Version)
	}
}

func TestParseMikanTitle_VersionInTitle(t *testing.T) {
	info := ParseMikanTitle("[VCB-Studio] Fate/stay night V2 1080p")
	if info.Version != 2 {
		t.Errorf("版本号 = %d, 期望 2", info.Version)
	}
}

func TestParseMikanTitle_IsBatch(t *testing.T) {
	info := ParseMikanTitle("【悠哈璃羽字幕社】孤独摇滚 全12集 合集")
	if !info.IsBatch {
		t.Error("应识别为合集")
	}
}

func TestParseMikanTitle_IsSpecial(t *testing.T) {
	info := ParseMikanTitle("[LoliHouse] 某科学的超电磁炮 OVA")
	if !info.IsSpecial {
		t.Error("应将 OVA 识别为特别篇")
	}
}

func TestParseMikanTitle_IsSP(t *testing.T) {
	info := ParseMikanTitle("葬送的芙莉莲 SP1")
	if !info.IsSpecial {
		t.Error("应将 SP 识别为特别篇")
	}
}

func TestParseMikanTitle_CleanCodecTags(t *testing.T) {
	info := ParseMikanTitle("[VCB-Studio] 攻壳机动队 [01] [Ma10p_1080p][x265_2flac]")
	if info.Episode != 1 {
		t.Errorf("集数 = %g, 期望 1", info.Episode)
	}
	// 标题不应包含编码标签
	if info.Title == "" {
		t.Error("番剧名为空")
	}
}
