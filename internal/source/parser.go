// Package source 实现各资源站点的 Source 接口
// Mikan 标题解析器 — 从 Mikan 种子标题中提取元数据
package source

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
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

// SetCustomRegexPatterns 更新设置预览和调度器共用的规则。
func SetCustomRegexPatterns(patterns []string) error { return parser.SetCustomRegexSlots(patterns) }

// GetCustomRegexPatterns 返回已配置的规则。
func GetCustomRegexPatterns() []string {
	var result []string
	for _, pattern := range parser.GetCustomRegexSlots() {
		if pattern != "" {
			result = append(result, pattern)
		}
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
			continue
		}
		patterns = append(patterns, strings.TrimSpace(val))
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

	// 使用统一解析器返回的集号；旧格式仅用于从片名中移除集号及发布标记。
	detailed := parser.ExtractEpisodeDetailed(rawTitle)
	info.Season, info.Episode = detailed.Season, detailed.Episode
	info.Resolution, info.Version = detailed.Resolution, detailed.Version
	info.IsSpecial, info.IsBatch = detailed.IsSpecial, detailed.IsBatch
	if detailed.MatchedBy == "custom" {
		title = strings.Replace(title, detailed.MatchedToken, "", 1)
	} else {
		for _, pattern := range reEpisodePatterns {
			if pattern.MatchString(title) {
				title = pattern.ReplaceAllString(title, "")
				break
			}
		}
	}

	// 全面清理：去除分辨率、编码、字幕组等常见标签
	title = reCleanTags.ReplaceAllString(title, "")
	title = reResolution.ReplaceAllString(title, "")
	title = reVersion.ReplaceAllString(title, "")
	if info.IsSpecial {
		title = reSpecial.ReplaceAllString(title, "")
	}
	if info.IsBatch || detailed.IsEnd {
		title = reBatch.ReplaceAllString(title, "")
	}

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

// ParseMikanTitleAdvanced 在本地正则的基础上，利用放送时间戳与 Bangumi 单集日历进行二阶段零成本对齐
func ParseMikanTitleAdvanced(rawTitle string, pubDate time.Time, episodes []core.Episode) TitleInfo {
	info := ParseMikanTitle(rawTitle)
	if parser.ExtractEpisodeDetailed(rawTitle).MatchedBy == "custom" {
		return info
	}
	if cached, ok := parser.GetCachedTitleResult(rawTitle); ok && cached.ResolvedBy == "airdate" && len(episodes) == 0 {
		info.Season, info.Episode = cached.Season, cached.Episode
		return info
	}

	// 第二级：若有放送时间戳与单集日历，进行确定性对齐校准
	if !pubDate.IsZero() && len(episodes) > 0 {
		match := parser.MatchEpisodeByAirDate(pubDate, rawTitle, episodes)
		if match.Matched {
			info.Season = match.Season
			info.Episode = match.Episode
			_ = parser.SaveCachedTitleResult(rawTitle, info.Title, info.Season, info.Episode, info.Subgroup, info.Resolution, match.Confidence, "airdate")
			return info
		}
	}

	return info
}
