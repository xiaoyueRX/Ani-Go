package parser

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestExtractEpisode_YearVsEpisode 测试年份与分辨率数字不应误判为集数
func TestExtractEpisode_YearVsEpisode(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		wantSeason  int
		wantEpisode float32
	}{
		{
			name:        "年份与集数同时出现",
			title:       "[NC-Raws] 某科学的超电磁炮 (2020) [05] [1080p]",
			wantSeason:  1,
			wantEpisode: 5,
		},
		{
			name:        "日期与连字符集数",
			title:       "【幻樱字幕组】[2024.04.06] 葬送的芙莉莲 - 03 [1080p HEVC]",
			wantSeason:  1,
			wantEpisode: 3,
		},
		{
			name:        "1080p分辨率不被识别为集数",
			title:       "[Lilith-Raws] 鬼灭之刃 第08集 [1080p]",
			wantSeason:  1,
			wantEpisode: 8,
		},
		{
			name:        "10bit与720p不干扰集数",
			title:       "[DMG] 进击的巨人 - 12 [720p HEVC-10bit]",
			wantSeason:  1,
			wantEpisode: 12,
		},
		{
			name:        "CRC32十六进制数字不干扰",
			title:       "[SubGroup] Anime Title - 04 [1080p][A1B2C3D4].mkv",
			wantSeason:  1,
			wantEpisode: 4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, ep := ExtractEpisode(tc.title)
			if s != tc.wantSeason || ep != tc.wantEpisode {
				t.Errorf("ExtractEpisode(%q) = (S%d, E%.1f), 期望 (S%d, E%.1f)", tc.title, s, ep, tc.wantSeason, tc.wantEpisode)
			}
		})
	}
}

// TestExtractEpisode_MultiSeason 测试多季标记与冲突季号清洗
func TestExtractEpisode_MultiSeason(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		wantSeason  int
		wantEpisode float32
	}{
		{
			name:        "S02E05格式",
			title:       "[SweetSub] 间谍过家家 S02E05 [1080P]",
			wantSeason:  2,
			wantEpisode: 5,
		},
		{
			name:        "Season 2 Episode 10",
			title:       "[Subs] Dungeon Meshi Season 2 Episode 10 [1080p]",
			wantSeason:  2,
			wantEpisode: 10,
		},
		{
			name:        "2nd Season 序数词季号",
			title:       "[Raws] Jujutsu Kaisen 2nd Season - 03 [1080p]",
			wantSeason:  2,
			wantEpisode: 3,
		},
		{
			name:        "中文第2季",
			title:       "【喵萌奶茶屋】进击的巨人 第2季 第04话",
			wantSeason:  2,
			wantEpisode: 4,
		},
		{
			name:        "多重冲突季号标记优先提取",
			title:       "间谍过家家 Season 2 S02 第2季 第二季 - 01 [1080p]",
			wantSeason:  2,
			wantEpisode: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, ep := ExtractEpisode(tc.title)
			if s != tc.wantSeason || ep != tc.wantEpisode {
				t.Errorf("ExtractEpisode(%q) = (S%d, E%.1f), 期望 (S%d, E%.1f)", tc.title, s, ep, tc.wantSeason, tc.wantEpisode)
			}
		})
	}
}

// TestExtractEpisode_VersionsAndHalfEpisodes 测试 v2/v3 版本号与 .5 小数集
func TestExtractEpisode_VersionsAndHalfEpisodes(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		wantSeason  int
		wantEpisode float32
		wantVersion int
	}{
		{
			name:        "方括号带v2",
			title:       "[LoliHouse] 葬送的芙莉莲 [01v2] [1080p]",
			wantSeason:  1,
			wantEpisode: 1,
			wantVersion: 2,
		},
		{
			name:        "连字符带v3",
			title:       "[Sakurato] Steel Ball Run - 12v3 [1080p]",
			wantSeason:  1,
			wantEpisode: 12,
			wantVersion: 3,
		},
		{
			name:        "SxxExx带v2",
			title:       "[Raws] Mushoku Tensei S02E04v2 [1080p]",
			wantSeason:  2,
			wantEpisode: 4,
			wantVersion: 2,
		},
		{
			name:        "点5小数集",
			title:       "[SweetSub] 咒术回战 [02.5] [1080p]",
			wantSeason:  1,
			wantEpisode: 2.5,
			wantVersion: 0,
		},
		{
			name:        "中文点5集数",
			title:       "【幻樱】某科学的超电磁炮 第08.5集 [720p]",
			wantSeason:  1,
			wantEpisode: 8.5,
			wantVersion: 0,
		},
		{
			name:        "SxxExx带点5集数",
			title:       "[Subs] Anime S01E06.5 [1080p]",
			wantSeason:  1,
			wantEpisode: 6.5,
			wantVersion: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := ExtractEpisodeDetailed(tc.title)
			if info.Season != tc.wantSeason || info.Episode != tc.wantEpisode || info.Version != tc.wantVersion {
				t.Errorf("ExtractEpisodeDetailed(%q) = (S%d, E%.1f, v%d), 期望 (S%d, E%.1f, v%d)",
					tc.title, info.Season, info.Episode, info.Version, tc.wantSeason, tc.wantEpisode, tc.wantVersion)
			}
		})
	}
}

// TestExtractEpisode_SpecialsAndEnd 测试 OVA/SP/OAD 与 END 标记
func TestExtractEpisode_SpecialsAndEnd(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		wantSeason  int
		wantEpisode float32
		wantSpecial bool
		wantEnd     bool
	}{
		{
			name:        "独立SP01特番",
			title:       "[DBD-Raws] 某科学的超电磁炮 [SP01] [1080p]",
			wantSeason:  0,
			wantEpisode: 1,
			wantSpecial: true,
			wantEnd:     false,
		},
		{
			name:        "OVA带集数",
			title:       "[LoliHouse] 进击的巨人 OVA 02 [1080p]",
			wantSeason:  0,
			wantEpisode: 2,
			wantSpecial: true,
			wantEnd:     false,
		},
		{
			name:        "OAD带集数",
			title:       "[Raws] 鬼灭之刃 OAD01 [1080p]",
			wantSeason:  0,
			wantEpisode: 1,
			wantSpecial: true,
			wantEnd:     false,
		},
		{
			name:        "方括号带END结尾",
			title:       "[SweetSub] 迷宫饭 [24 END] [1080p]",
			wantSeason:  1,
			wantEpisode: 24,
			wantSpecial: false,
			wantEnd:     true,
		},
		{
			name:        "连字符带完结标记",
			title:       "【幻樱】葬送的芙莉莲 - 28 完 [1080p]",
			wantSeason:  1,
			wantEpisode: 28,
			wantSpecial: false,
			wantEnd:     true,
		},
		{
			name:        "SxxExx带FIN标记",
			title:       "[Subs] Anime S01E12 FIN [1080p]",
			wantSeason:  1,
			wantEpisode: 12,
			wantSpecial: false,
			wantEnd:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := ExtractEpisodeDetailed(tc.title)
			if info.Season != tc.wantSeason || info.Episode != tc.wantEpisode || info.IsSpecial != tc.wantSpecial || info.IsEnd != tc.wantEnd {
				t.Errorf("ExtractEpisodeDetailed(%q) = (S%d, E%.1f, SP=%v, END=%v), 期望 (S%d, E%.1f, SP=%v, END=%v)",
					tc.title, info.Season, info.Episode, info.IsSpecial, info.IsEnd, tc.wantSeason, tc.wantEpisode, tc.wantSpecial, tc.wantEnd)
			}
		})
	}
}

// TestExtractEpisode_BatchRanges 测试合集范围排除单集
func TestExtractEpisode_BatchRanges(t *testing.T) {
	batchTitles := []string{
		"[喵萌奶茶屋] [我独自升级] [01-12] [720p] [繁体] [MP4]",
		"[DBD-Raws] [SP01-04] [1080P] [BDRip] [HEVC-10bit]",
		"【幻樱】葬送的芙莉莲 第01-24集 [1080p]",
		"[SweetSub] 迷宫饭 全24集 [1080p]",
		"【字幕组】进击的巨人 最终季 合集 1080p",
	}

	for _, title := range batchTitles {
		t.Run(title, func(t *testing.T) {
			info := ExtractEpisodeDetailed(title)
			if !info.IsBatch {
				t.Errorf("期望 IsBatch=true, 实际: %+v", info)
			}
			if info.Episode != 0 {
				t.Errorf("合集集数期望为 0, 实际为: %.1f", info.Episode)
			}
		})
	}

	// 验证年份范围（如 [2023-2024]）不被误判为合集
	nonBatch := "[Sub] Anime (2023-2024) [01] [1080p]"
	infoNonBatch := ExtractEpisodeDetailed(nonBatch)
	if infoNonBatch.IsBatch {
		t.Errorf("年份范围标题不应判为合集: %s", nonBatch)
	}
	if infoNonBatch.Episode != 1 {
		t.Errorf("集数期望为 1, 实际为: %.1f", infoNonBatch.Episode)
	}
}

// TestCustomRegexSlots 测试自定义正则槽位配置、提取与边界防护
func TestCustomRegexSlots(t *testing.T) {
	defer ClearCustomRegexSlots()

	// 1. 设置单个槽位（提取集数）
	err := SetCustomRegexSlot(0, `【自定义】(?:EP)?(\d+)`)
	if err != nil {
		t.Fatalf("SetCustomRegexSlot 失败: %v", err)
	}

	// 2. 槽位越界保护
	if err := SetCustomRegexSlot(-1, ".*"); err == nil {
		t.Errorf("负数槽位索引应报错")
	}
	if err := SetCustomRegexSlot(10, ".*"); err == nil {
		t.Errorf("超出槽位上限应报错")
	}

	// 3. 正则无捕获组保护
	if err := SetCustomRegexSlot(1, `\d+`); err == nil {
		t.Errorf("缺少捕获组的正效应报错")
	}

	// 4. 超长正则防护
	longPattern := `(` + strings.Repeat("a", 600) + `)`
	if err := SetCustomRegexSlot(1, longPattern); err == nil {
		t.Errorf("超长正则应被截断拒绝")
	}

	// 5. 验证槽位命中
	info := ExtractEpisodeDetailed("【字幕组】【自定义】EP07 1080p")
	if info.Episode != 7 || info.MatchedBy != "custom" {
		t.Errorf("自定义槽位未正确生效: %+v", info)
	}

	// 6. 支持双捕获组 (Season + Episode)
	err = SetCustomRegexSlot(1, `(?i)CustomS(\d+)E(\d+)`)
	if err != nil {
		t.Fatalf("设置双捕获组失败: %v", err)
	}
	info2 := ExtractEpisodeDetailed("[Group] CustomS03E09 [1080p]")
	if info2.Season != 3 || info2.Episode != 9 || info2.MatchedBy != "custom" {
		t.Errorf("双捕获组自定义槽位未正确生效: %+v", info2)
	}

	// 7. 槽位清空
	if err := SetCustomRegexSlot(0, ""); err != nil {
		t.Fatalf("清空槽位失败: %v", err)
	}
	slots := GetCustomRegexSlots()
	if slots[0] != "" {
		t.Errorf("槽位 0 期望为空，实际为 %q", slots[0])
	}

	// 8. LoadCustomRegexSlots 模拟
	mockSettings := map[string]string{
		"custom_regex_0": `\[MyRule\]\s*E(\d+)`,
		"custom_regex_1": `\[Rule2\]\s*S(\d+)E(\d+)`,
	}
	err = LoadCustomRegexSlots(func(k string) (string, bool) {
		val, ok := mockSettings[k]
		return val, ok
	})
	if err != nil {
		t.Fatalf("LoadCustomRegexSlots 失败: %v", err)
	}
	info3 := ExtractEpisodeDetailed("[Group] [MyRule] E15 [1080p]")
	if info3.Episode != 15 || info3.MatchedBy != "custom" {
		t.Errorf("通过 LoadCustomRegexSlots 加载的规则匹配失败: %+v", info3)
	}
}

// TestRegexParser_ContextAndBounds 测试正则解析器的上下文超时与边界保护
func TestRegexParser_ContextAndBounds(t *testing.T) {
	p := NewRegexParser()

	// 已取消的 Context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Parse(ctx, "追番 进击的巨人")
	if err == nil {
		t.Errorf("期望返回 context canceled 错误，实际为 nil")
	}

	// 超长与全控制字符输入
	dirty := "\x00\u200B\u202E" + strings.Repeat(" ", 500) + "订阅 鬼灭之刃 第4季 1080p"
	res, err := p.Parse(context.Background(), dirty)
	if err != nil {
		t.Fatalf("解析脏输入失败: %v", err)
	}
	if res.Title != "鬼灭之刃" || res.Season != 4 || res.Resolution != "1080p" {
		t.Errorf("清洗后解析结果异常: %+v", res)
	}
}

// TestCustomRegex_Concurrency 测试高并发读写自定义正则槽位无数据竞争
func TestCustomRegex_Concurrency(t *testing.T) {
	defer ClearCustomRegexSlots()

	var wg sync.WaitGroup
	const workers = 15

	for w := 0; w < workers; w++ {
		wg.Add(1)
		workerID := w
		go func() {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				if workerID%2 == 0 {
					_ = SetCustomRegexSlot(workerID%maxCustomRegexSlots, fmt.Sprintf(`\[Group%d\]\s*(\d+)`, workerID))
				} else {
					_ = GetCustomRegexSlots()
					_ = ExtractEpisodeDetailed(fmt.Sprintf("[Group%d] 08 1080p", workerID))
				}
			}
		}()
	}

	wg.Wait()
}
