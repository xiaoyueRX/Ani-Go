// Package source 实现各资源站点的 Source 接口
// Mikan 标题解析器 — 从 Mikan 种子标题中提取元数据
package source

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ============================================================
// 标题解析器 — 从 Mikan 种子标题中提取元数据
// ============================================================

// TitleInfo 保存从种子标题中解析出的元数据
type TitleInfo struct {
	Title      string  // 番剧名（不含字幕组、集数、分辨率）
	RawTitle   string  // 原始标题
	Subgroup   string  // 字幕组名称
	Episode    float32 // 集数（0 表示未识别）
	Season     int     // 季数（默认 1）
	InfoHash   string  // info hash（如有）
	Resolution string  // 分辨率，如 "1080p"
	Version    int     // 版本号（v2 → 2，0 表示无版本标识）
	IsSpecial  bool    // 是否为特别篇/OVA
	IsBatch    bool    // 是否为合集
}

// 用于解析 Mikan 标题的正则表达式集合
// 参考 ani-rss RenameUtil.REG_STR 的设计
var (
	// 提取【】[] 中的字幕组名称
	reSubgroup = regexp.MustCompile(`^[【\[［]([^】\]］]+)[】\]］]\s*`)

	// 集数模式（按优先级排列）
		reEpisodePatterns = []*regexp.Regexp{
			// SxxExx 格式优先匹配
			regexp.MustCompile(`[Ss](?:eason)?\s*(\d{1,2})\s*[Ee](?:p(?:isode)?)?\s*(\d{1,3})(?:\.5)?`),
			// "- 01"、" 01" 结尾（含可选 END/FIN/完标记）
			regexp.MustCompile(`[-\s](\d{1,3})(?:\.5)?(?:\s*\(\d+\))?(?:\s*(?:END|end|Fin|fin|完))?\s*(?:$|[\[【])`),
			// "Vol 5" 卷数
			regexp.MustCompile(`[Vv]ol\s*(\d{1,3})(?:\.5)?`),
			// "第01話/话/集"
			regexp.MustCompile(`第(\d{1,3})(?:\.5)?[話话集]`),
			// "EP01"、"E01"
			regexp.MustCompile(`[Ee][Pp]?\s*(\d{1,3})(?:\.5)?`),
			// "#01"
			regexp.MustCompile(`#(\d{1,3})(?:\.5)?`),
			// 【01】中文方括号集数
					regexp.MustCompile(`【(\d{1,3})(?:\.5)?】`),
			// [01] 英文方括号集数（含可选版本号和 END 标记）
			regexp.MustCompile(`\[(\d{1,3})(?:\.5)?(?:\s*\(\d+\))?(?:\s*[vV](\d))?(?:\s*(?:END|end|Fin|fin|完))?\]`),
		}

	// 分辨率（参考 ani-rss 的 getResolution 方法）
	reResolution = regexp.MustCompile(`(?i)(\d{3,4}p)`)

	// 季数（参考 ani-rss StringEnum.SEASON_REG）
	reSeasonYear  = regexp.MustCompile(`[Ss](?:eason)?\s*(\d{1,2})`)
	reSeasonTitle = regexp.MustCompile(`第([\d一二三四五六七八九十]{1,3})季`)

	// 版本号 v2/v3
	reVersion = regexp.MustCompile(`(?i)[\s\[【][vV](\d)[\s\]】]?`)

	// 特别篇/OVA/SP
	reSpecial = regexp.MustCompile(`(?i)(OVA[Ds]?|SP|特别篇|特典|SP\s*\d|OAD|特別篇)`)

	// 合集/批量
	reBatch = regexp.MustCompile(`(?i)(合集|全(\d{1,3})集|Fin|END|完结)`)

	// 年份标记 (2024)
	reCleanTags = regexp.MustCompile(`(?i)\[?\s*(?:1080p|720p|2160p|4k|HEVC|AVC|AV1|H\.?264|H\.?265|x265|x264|AAC|FLAC|MKV|MP4|GB|BIG5|CH[TS]|简|繁|简繁|繁简|内[嵌封挂]|WebRip|BDRip|BD|Remux)\s*\]?\s*`)

	// 清理末尾纯数字
	reTrailingDigits = regexp.MustCompile(`\s+\d{1,3}\s*$`)
)

// 用户自定义正则模式（线程安全）
var (
	customRegexPatterns []*regexp.Regexp
	customRegexMu       sync.RWMutex
)

// SetCustomRegexPatterns 设置用户自定义正则模式
// 每个模式应包含一个捕获组提取集数（可选两个：Season + Episode 类似 SxxExx）
func SetCustomRegexPatterns(patterns []string) error {
	customRegexMu.Lock()
	defer customRegexMu.Unlock()

	customRegexPatterns = make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("自定义正则编译失败: %s → %w", p, err)
		}
		customRegexPatterns = append(customRegexPatterns, re)
	}
	if len(customRegexPatterns) > 0 {
		log.Printf("✅ 已加载 %d 条自定义正则解析规则", len(customRegexPatterns))
	}
	return nil
}

// GetCustomRegexPatterns 返回当前自定义正则模式（用于展示）
func GetCustomRegexPatterns() []string {
	customRegexMu.RLock()
	defer customRegexMu.RUnlock()

	result := make([]string, len(customRegexPatterns))
	for i, re := range customRegexPatterns {
		result[i] = re.String()
	}
	return result
}

// LoadCustomPatternsFromSettings 从数据库 settings 表加载自定义正则
// 格式：custom_regex_0, custom_regex_1, ...
func LoadCustomPatternsFromSettings(getSetting func(key string) (string, bool)) {
	var patterns []string
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("custom_regex_%d", i)
		val, ok := getSetting(key)
		if !ok || strings.TrimSpace(val) == "" {
			break
		}
		patterns = append(patterns, val)
	}
	if err := SetCustomRegexPatterns(patterns); err != nil {
		log.Printf("⚠️  加载自定义正则失败: %v", err)
	}
}

// ParseMikanTitle 从 Mikan 种子标题中提取结构化信息
// 参考 ani-rss RenameUtil.rename() 的解析逻辑
// 优先尝试用户自定义正则，再回退到内置 8 种模式
func ParseMikanTitle(rawTitle string) TitleInfo {
	info := TitleInfo{
		RawTitle: rawTitle,
		Season:   1,
	}

	title := strings.TrimSpace(rawTitle)

	// 提取字幕组
	if m := reSubgroup.FindStringSubmatch(title); m != nil {
		info.Subgroup = strings.TrimSpace(m[1])
		title = strings.TrimSpace(reSubgroup.ReplaceAllString(title, ""))
	}

	// 检测特别篇/OVA
	if reSpecial.MatchString(title) {
		info.IsSpecial = true
	}

	// 检测合集
	if reBatch.MatchString(title) {
		info.IsBatch = true
	}

	// 提取分辨率
	if m := reResolution.FindStringSubmatch(title); m != nil {
		info.Resolution = m[1]
	}

	// 提取版本号 v2/v3
	if m := reVersion.FindStringSubmatch(title); m != nil {
		info.Version, _ = strconv.Atoi(m[1])
	}

	// 先尝试用户自定义正则（优先级高于内置模式）
	customRegexMu.RLock()
	customPats := make([]*regexp.Regexp, len(customRegexPatterns))
	copy(customPats, customRegexPatterns)
	customRegexMu.RUnlock()

	var matched bool
	for i, re := range customPats {
		m := re.FindStringSubmatch(title)
		if m == nil {
			continue
		}
		// 捕获组 >= 3：(1=Season, 2=Episode)；否则 1=Episode
		if len(m) >= 3 {
			if s, err := strconv.Atoi(m[1]); err == nil && s > 0 {
				info.Season = s
			}
			if ep, err := strconv.ParseFloat(m[2], 32); err == nil {
				info.Episode = float32(ep)
			}
		} else {
			if ep, err := strconv.ParseFloat(m[1], 32); err == nil {
				info.Episode = float32(ep)
			}
		}
		if strings.Contains(m[0], ".5") {
			info.Episode += 0.5
		}
		title = re.ReplaceAllString(title, "")
		log.Printf("🔧 自定义正则命中 [%d]: %s → S%dE%.1f", i, m[0], info.Season, info.Episode)
		matched = true
		break
	}

	// 逐模式尝试提取集数（内置 8 种模式）
	if !matched {
		for ri, re := range reEpisodePatterns {
			m := re.FindStringSubmatch(title)
			if m == nil {
				continue
			}

			// patterns[0] 是 SxxExx 格式（m[1]=Season, m[2]=Episode）
			if ri == 0 {
				if s, err := strconv.Atoi(m[1]); err == nil && s > 0 {
					info.Season = s
				}
				if ep, err := strconv.ParseFloat(m[2], 32); err == nil {
					info.Episode = float32(ep)
				}
			} else {
				// 其他格式：m[1] = 集数
				if ep, err := strconv.ParseFloat(m[1], 32); err == nil {
					info.Episode = float32(ep)
				}
			}

			// 检测 .5 集数 (仅当当前集数为整数时才加 0.5，避免从自定义正则或模式匹配中双重计算)
			if strings.Contains(m[0], ".5") && info.Episode == float32(int(info.Episode)) {
				info.Episode += 0.5
			}

			// 检测版本号（后续捕获组可能含版本号，如 [01v2] 的 m[2]="2"）
			for i := 2; i < len(m); i++ {
				if v, err := strconv.Atoi(m[i]); err == nil && v >= 2 && v <= 9 && info.Version == 0 {
					info.Version = v
				}
			}

			title = re.ReplaceAllString(title, "")
			break
		}
	}

	// 提取季数（从 "Season 2" 或 "第二季" 关键词）
	if m := reSeasonYear.FindStringSubmatch(title); m != nil {
		season, _ := strconv.Atoi(m[1])
		if season > info.Season {
			info.Season = season
		}
	}
	if m := reSeasonTitle.FindStringSubmatch(title); m != nil {
		info.Season = parseCNNumber(m[1])
	}

	// 全面清理：去除分辨率、编码、字幕组等常见标签
	title = reCleanTags.ReplaceAllString(title, "")
	title = reResolution.ReplaceAllString(title, "")
	title = reVersion.ReplaceAllString(title, "")
	title = reSpecial.ReplaceAllString(title, "")
	title = reBatch.ReplaceAllString(title, "")

	// 清理杂项字符
	title = strings.TrimSpace(title)
	title = strings.TrimRight(title, "- _[(（/[]")
	title = strings.TrimSpace(title)

	// 移除末尾可能残留的纯数字
	if reTrailingDigits.MatchString(title) {
		title = reTrailingDigits.ReplaceAllString(title, "")
	}

	info.Title = strings.TrimSpace(title)
	return info
}

// ============================================================
// 辅助函数
// ============================================================

// parseCNNumber 将中文数字字符串转为阿拉伯数字
func parseCNNumber(s string) int {
	// 先尝试直接解析阿拉伯数字
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	cnMap := map[rune]int{
		'一': 1, '二': 2, '三': 3, '四': 4, '五': 5,
		'六': 6, '七': 7, '八': 8, '九': 9, '十': 10,
	}
	// 处理 "十一" ~ "十九"
	if len([]rune(s)) == 2 {
		runes := []rune(s)
		if runes[0] == '十' {
			return 10 + cnMap[runes[1]]
		}
		if runes[1] == '十' {
			return cnMap[runes[0]] * 10
		}
	}
	// 处理 "二十" ~ "九十九"
	if len([]rune(s)) == 3 {
		runes := []rune(s)
		if runes[1] == '十' {
			return cnMap[runes[0]]*10 + cnMap[runes[2]]
		}
	}
	// 单个中文数字
	if n, ok := cnMap[[]rune(s)[0]]; ok {
		return n
	}
	return 1
}