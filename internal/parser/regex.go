package parser

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

const (
	// maxParserInputRunes 限制单次正则解析的最大字符数，防止超长脏输入引发性能退化（ReDoS 防护）
	maxParserInputRunes = 2048
	// maxCNNumRunes 限制中文季数解析的最大字符数，防止异常输入导致整数溢出
	maxCNNumRunes = 16
	// maxCustomRegexSlots 自定义正则槽位上限（对应 custom_regex_0 ~ custom_regex_9）
	maxCustomRegexSlots = 10
	// maxCustomRegexLen 单条自定义正则的最大长度限制
	maxCustomRegexLen = 512
	// maxSeasonNumber 合法季号上限
	maxSeasonNumber = 99
	// maxEpisodeNumber 合法集数上限（兼容超长连载番剧，同时过滤年份）
	maxEpisodeNumber = 2000
)

var (
	reAction     = regexp.MustCompile(`^(追番|订阅|取消订阅|退订|查看|列出|搜索|查)`)
	reSeasonNum  = regexp.MustCompile(`第\s*(\d{1,2})\s*季`)
	reSeasonCN   = regexp.MustCompile(`第\s*([一二三四五六七八九十]{1,4})\s*季`)
	reSeasonS    = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(S(?:eason)?\s*0*(\d{1,2}))(?:$|[^A-Za-z0-9])`)
	reSeasonOrd  = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(\d{1,2})(?:st|nd|rd|th)\s+Season(?:$|[^A-Za-z0-9])`)
	reResolution = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(2160p|1440p|1080p|720p|480p|360p|4[kK])(?:$|[^A-Za-z0-9])`)
	reEpisode    = regexp.MustCompile(`第\s*(\d{1,4}(?:\.\d+)?)\s*[集話话]`)
	reYear       = regexp.MustCompile(`\((?:19|20)\d{2}\)|（(?:19|20)\d{2}）`)

	// 种子标题多季/特别篇/版本号/合集识别正则
	reTorrentSxxExx = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])S(?:eason)?\s*0*(\d{1,2})\s*[Ee](?:p(?:isode)?)?\s*0*(\d{1,4}(?:\.5)?)(?:\s*[vV](\d))?(?:\s*(?:END|FIN|完))?(?:$|[^A-Za-z0-9])`)
	reTorrentSeason = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(?:S|Season)\s*0*(\d{1,2})(?:$|[^A-Za-z0-9])`)
	reTorrentSpecial = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(OVA[Ds]?|OAD|OAV|SP(?:\s*0*\d{1,3})?|Special|特别篇|特別篇|特典)(?:$|[^A-Za-z0-9])`)
	reTorrentSpecialNum = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(?:OVA|OAD|OAV|SP)\s*0*(\d{1,3}(?:\.5)?)(?:\s*[vV](\d))?(?:\s*(?:END|FIN|完))?(?:$|[^A-Za-z0-9])`)
	reTorrentVersion = regexp.MustCompile(`(?i)(?:^|[\s\d\[【])[vV](\d)(?:$|[\s\]】\)])`)
	reTorrentEnd     = regexp.MustCompile(`(?i)(?:^|[\s\d\[【\(])(?:END|FIN|完结|完)(?:$|[\s\]】\)])`)

	// 合集/范围匹配（如 [01-12], [01~24], [SP01-04], 第01-12集, 全12集, 合集）
	reTorrentBatchBracket = regexp.MustCompile(`(?i)[\[\(【（]\s*(?:SP|OVA|OAD|EP?)?\s*0*(\d{1,3})\s*[-~～]\s*0*(\d{1,3})(?:\s*(?:END|FIN|完|[vV]\d))?\s*[\]\)】）]`)
	reTorrentBatchCN      = regexp.MustCompile(`(?:第\s*0*(\d{1,3})\s*[-~～]\s*0*(\d{1,3})\s*[集话話]|全\s*0*(\d{1,3})\s*[集话話]|合集)`)

	// 清洗非集数数字干扰（年份、日期、月新番、纯数字分辨率如 [1080]、CRC32 校验码）
	reNoiseYearDate   = regexp.MustCompile(`(?i)(?:[\[\(【（]\s*(?:19|20)\d{2}\s*[\]\)】）]|(?:19|20)\d{2}[-./]\d{1,2}(?:[-./]\d{1,2})?|\d{1,2}月新番)`)
	reNoiseCRC32      = regexp.MustCompile(`\[[0-9A-Fa-f]{8}\]`)
	reNoiseBracketRes = regexp.MustCompile(`[\[\(【（]\s*(?:2160|1440|1080|720|480|360)\s*[\]\)】）]`)

	// 按优先级排列的种子单集集数正则
	reTorrentEpChinese  = regexp.MustCompile(`(?i)第\s*0*(\d{1,4}(?:\.5)?)\s*[集话話](?:\s*[vV](\d))?`)
	reTorrentEpPrefix   = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(?:EP?|Episode)\s*0*(\d{1,4}(?:\.5)?)(?:\s*[vV](\d))?(?:\s*(?:END|FIN|完))?(?:$|[^A-Za-z0-9])`)
	reTorrentEpBracket  = regexp.MustCompile(`(?i)[\[【]\s*0*(\d{1,4}(?:\.5)?)(?:\s*\(\d+\))?(?:\s*[vV](\d))?(?:\s*(?:\(?END\)?|\(?FIN\)?|完))?\s*[\]】]`)
	reTorrentEpDash     = regexp.MustCompile(`(?i)(?:^|\s)-\s*0*(\d{1,4}(?:\.5)?)(?:\s*\(\d+\))?(?:\s*[vV](\d))?(?:\s*(?:\(?END\)?|\(?FIN\)?|完))?\s*(?:$|[\[\(【（\s]|END|FIN|完|[vV]\d|\.\w{2,4}$)`)
	reTorrentEpHashVol  = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(?:#|Vol\.?\s*)0*(\d{1,4}(?:\.5)?)(?:\s*[vV](\d))?(?:$|[^A-Za-z0-9])`)
	reTorrentEpVerToken = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])0*(\d{1,4}(?:\.5)?)[vV](\d)(?:$|[^A-Za-z0-9])`)

	reSpaces   = regexp.MustCompile(`\s+`)
	reCleanRes = regexp.MustCompile(`(?i)\b(2160p|1440p|1080p|720p|480p|360p|4[kK])\b`)
)

// 自定义正则槽位管理（固定 10 个槽位，读写受读写锁保护）
var (
	customSlotMu    sync.RWMutex
	customSlotRegex [maxCustomRegexSlots]*regexp.Regexp
)

var cnNum = map[rune]int{
	'一': 1, '二': 2, '三': 3, '四': 4, '五': 5,
	'六': 6, '七': 7, '八': 8, '九': 9, '十': 10,
}

// TorrentEpisodeInfo 种子标题结构化解析详细信息
type TorrentEpisodeInfo struct {
	Season     int
	Episode    float32
	Version    int
	IsSpecial  bool
	IsBatch    bool
	IsEnd      bool
	Resolution string
	MatchedBy  string
}

type RegexParser struct{}

func NewRegexParser() *RegexParser { return &RegexParser{} }

func (p *RegexParser) Name() string { return "regex" }

// SetCustomRegexSlots 批量验证并更新自定义正则槽位（最多 10 条，原子替换避免部分生效）
func SetCustomRegexSlots(patterns []string) error {
	if len(patterns) > maxCustomRegexSlots {
		return fmt.Errorf("自定义正则槽位数量超出上限 (%d)", maxCustomRegexSlots)
	}

	var compiled [maxCustomRegexSlots]*regexp.Regexp
	for i, raw := range patterns {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if len(p) > maxCustomRegexLen {
			return fmt.Errorf("槽位 %d 正则长度超出限制 (%d)", i, maxCustomRegexLen)
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("槽位 %d 正则编译失败: %w", i, err)
		}
		if re.NumSubexp() < 1 {
			return fmt.Errorf("槽位 %d 正则至少需要包含 1 个捕获组用于提取集数", i)
		}
		compiled[i] = re
	}

	customSlotMu.Lock()
	customSlotRegex = compiled
	customSlotMu.Unlock()
	return nil
}

// SetCustomRegexSlot 设置单个自定义正则槽位（空字符串表示清空该槽位）
func SetCustomRegexSlot(slot int, pattern string) error {
	if slot < 0 || slot >= maxCustomRegexSlots {
		return fmt.Errorf("自定义正则槽位索引越界: %d (有效范围 0~%d)", slot, maxCustomRegexSlots-1)
	}
	p := strings.TrimSpace(pattern)
	if p == "" {
		customSlotMu.Lock()
		customSlotRegex[slot] = nil
		customSlotMu.Unlock()
		return nil
	}
	if len(p) > maxCustomRegexLen {
		return fmt.Errorf("槽位 %d 正则长度超出限制 (%d)", slot, maxCustomRegexLen)
	}
	re, err := regexp.Compile(p)
	if err != nil {
		return fmt.Errorf("槽位 %d 正则编译失败: %w", slot, err)
	}
	if re.NumSubexp() < 1 {
		return fmt.Errorf("槽位 %d 正则至少需要包含 1 个捕获组用于提取集数", slot)
	}

	customSlotMu.Lock()
	customSlotRegex[slot] = re
	customSlotMu.Unlock()
	return nil
}

// GetCustomRegexSlots 返回当前生效的自定义正则槽位列表（固定长度 10）
func GetCustomRegexSlots() []string {
	customSlotMu.RLock()
	defer customSlotMu.RUnlock()

	out := make([]string, maxCustomRegexSlots)
	for i, re := range customSlotRegex {
		if re != nil {
			out[i] = re.String()
		}
	}
	return out
}

// ClearCustomRegexSlots 清空所有自定义正则槽位
func ClearCustomRegexSlots() {
	customSlotMu.Lock()
	customSlotRegex = [maxCustomRegexSlots]*regexp.Regexp{}
	customSlotMu.Unlock()
}

// LoadCustomRegexSlots 从配置读取器加载 custom_regex_0 ~ custom_regex_9
func LoadCustomRegexSlots(getSetting func(key string) (string, bool)) error {
	if getSetting == nil {
		return nil
	}
	patterns := make([]string, maxCustomRegexSlots)
	for i := 0; i < maxCustomRegexSlots; i++ {
		key := fmt.Sprintf("custom_regex_%d", i)
		if val, ok := getSetting(key); ok {
			patterns[i] = strings.TrimSpace(val)
		}
	}
	return SetCustomRegexSlots(patterns)
}

func snapshotCustomRegexSlots() []*regexp.Regexp {
	customSlotMu.RLock()
	defer customSlotMu.RUnlock()

	var list []*regexp.Regexp
	for _, re := range customSlotRegex {
		if re != nil {
			list = append(list, re)
		}
	}
	return list
}

func (p *RegexParser) Parse(ctx context.Context, input string) (core.ParseResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return core.ParseResult{RawInput: input}, err
		}
	}

	trimmed := strings.TrimSpace(input)
	result := core.ParseResult{RawInput: trimmed, Season: 1, Confidence: 0.5}

	// 限制进入正则管线的工作副本长度，保障 ReDoS 免疫与内存安全
	work := sanitizeControlRunes(trimmed)
	work = truncateRunes(work, maxParserInputRunes)
	work = strings.TrimSpace(work)

	actionMatch := reAction.FindStringSubmatch(work)
	if len(actionMatch) >= 2 {
		result.Action = mapAction(actionMatch[1])
		work = strings.TrimSpace(work[len(actionMatch[1]):])
	} else {
		result.Action = "subscribe"
		result.Confidence = 0.3
	}

	seasonMatched := false
	if m := reSeasonNum.FindStringSubmatch(work); len(m) >= 2 {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 0 && n <= maxSeasonNumber {
			result.Season = n
			result.Confidence += 0.1
			seasonMatched = true
		}
		work = strings.Replace(work, m[0], " ", 1)
	} else if m := reSeasonCN.FindStringSubmatch(work); len(m) >= 2 {
		result.Season = parseCNNumber(m[1])
		result.Confidence += 0.1
		seasonMatched = true
		work = strings.Replace(work, m[0], " ", 1)
	} else if m := reSeasonOrd.FindStringSubmatch(work); len(m) >= 2 {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 && n <= maxSeasonNumber {
			result.Season = n
			result.Confidence += 0.1
			seasonMatched = true
		}
		work = strings.Replace(work, m[0], " ", 1)
	} else if m := reSeasonS.FindStringSubmatch(work); len(m) >= 3 {
		if n, err := strconv.Atoi(m[2]); err == nil && n >= 0 && n <= maxSeasonNumber {
			result.Season = n
			result.Confidence += 0.1
			seasonMatched = true
		}
		work = strings.Replace(work, m[1], " ", 1)
	}

	// 若同时存在多重冲突季号标记（如 "Season 2 S02 第2季 第二季"），清理残余标记以保持标题干净
	if seasonMatched {
		work = reSeasonNum.ReplaceAllString(work, " ")
		work = reSeasonCN.ReplaceAllString(work, " ")
		work = reSeasonOrd.ReplaceAllString(work, " ")
		work = reSeasonS.ReplaceAllStringFunc(work, func(seg string) string {
			m := reSeasonS.FindStringSubmatch(seg)
			if len(m) >= 2 {
				return strings.Replace(seg, m[1], " ", 1)
			}
			return seg
		})
	}

	if m := reResolution.FindStringSubmatch(work); len(m) >= 2 {
		result.Resolution = strings.ToLower(m[1])
		result.Confidence += 0.05
		work = strings.Replace(work, m[1], " ", 1)
	}

	if m := reEpisode.FindStringSubmatch(work); m != nil {
		work = strings.Replace(work, m[0], " ", 1)
	}

	if m := reYear.FindStringSubmatch(work); m != nil {
		work = strings.Replace(work, m[0], " ", 1)
	}

	result.Title = cleanTitle(work)
	if result.Title != "" {
		result.Confidence += 0.2
	}

	if result.Title == "" {
		result.Title = strings.TrimSpace(work)
	}

	if result.Confidence > 1.0 {
		result.Confidence = 1.0
	}

	return result, nil
}

func mapAction(word string) string {
	switch word {
	case "追番", "订阅":
		return "subscribe"
	case "取消订阅", "退订":
		return "unsubscribe"
	case "查看", "列出":
		return "list"
	case "搜索", "查":
		return "search"
	}
	return "unknown"
}

func parseCNNumber(s string) int {
	s = truncateRunes(strings.TrimSpace(s), maxCNNumRunes)
	if s == "" {
		return 1
	}
	result := 0
	for _, r := range s {
		if v, ok := cnNum[r]; ok {
			if r == '十' {
				if result == 0 {
					result = 10
				} else {
					result *= v
				}
			} else {
				result += v
			}
			if result > maxSeasonNumber {
				return maxSeasonNumber
			}
		}
	}
	if result <= 0 {
		result = 1
	}
	return result
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimFunc(s, func(r rune) bool {
		return r == ' ' || r == '，' || r == ',' || r == '。' || r == '.'
	})
	s = reSpaces.ReplaceAllString(s, " ")
	s = reCleanRes.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = reSpaces.ReplaceAllString(s, " ")
	return s
}

// ExtractEpisode 尝试从种子标题中解析出季数和集数
func ExtractEpisode(title string) (season int, episode float32) {
	info := ExtractEpisodeDetailed(title)
	return info.Season, info.Episode
}

// ExtractEpisodeDetailed 从种子标题中提取季数、集数及版本/特别篇/合集等元信息
func ExtractEpisodeDetailed(title string) TorrentEpisodeInfo {
	info := TorrentEpisodeInfo{
		Season: 1, // 默认第一季
	}

	work := sanitizeControlRunes(strings.TrimSpace(title))
	work = truncateRunes(work, maxParserInputRunes)
	if work == "" {
		return info
	}

	if m := reResolution.FindStringSubmatch(work); len(m) >= 2 {
		info.Resolution = strings.ToLower(m[1])
	}
	if m := reTorrentVersion.FindStringSubmatch(work); len(m) >= 2 {
		if v, err := strconv.Atoi(m[1]); err == nil && v >= 2 && v <= 9 {
			info.Version = v
		}
	}
	if reTorrentEnd.MatchString(work) {
		info.IsEnd = true
	}
	if reTorrentSpecial.MatchString(work) {
		info.IsSpecial = true
	}

	// 先提取显式季号（英文 Sxx / Season x / 2nd Season / 中文第X季）
	explicitSeason := false
	if m := reTorrentSeason.FindStringSubmatch(work); len(m) >= 2 {
		if s, err := strconv.Atoi(m[1]); err == nil && s >= 0 && s <= maxSeasonNumber {
			info.Season = s
			explicitSeason = true
		}
	} else if m := reSeasonOrd.FindStringSubmatch(work); len(m) >= 2 {
		if s, err := strconv.Atoi(m[1]); err == nil && s >= 1 && s <= maxSeasonNumber {
			info.Season = s
			explicitSeason = true
		}
	} else if m := reSeasonNum.FindStringSubmatch(work); len(m) >= 2 {
		if s, err := strconv.Atoi(m[1]); err == nil && s >= 0 && s <= maxSeasonNumber {
			info.Season = s
			explicitSeason = true
		}
	} else if m := reSeasonCN.FindStringSubmatch(work); len(m) >= 2 {
		info.Season = parseCNNumber(m[1])
		explicitSeason = true
	}

	// 1. 最高优先级：用户自定义正则槽位（支持 1 捕获组=Episode 或 2 捕获组=Season+Episode）
	for _, re := range snapshotCustomRegexSlots() {
		m := re.FindStringSubmatch(work)
		if m == nil {
			continue
		}
		var parsedSeason int
		var parsedEp float32
		hasSeason := false
		if len(m) >= 3 {
			if s, err := strconv.Atoi(strings.TrimSpace(m[1])); err == nil && s >= 0 && s <= maxSeasonNumber {
				parsedSeason = s
				hasSeason = true
			}
			if ep, ok := parseValidEpisode(m[2]); ok {
				parsedEp = ep
			}
		} else if len(m) >= 2 {
			if ep, ok := parseValidEpisode(m[1]); ok {
				parsedEp = ep
			}
		}
		if parsedEp > 0 {
			if hasSeason {
				info.Season = parsedSeason
			}
			info.Episode = parsedEp
			info.MatchedBy = "custom"
			return info
		}
	}

	// 2. 显式 SxxExx 格式（如 S02E05、S02E05.5v2、Season 2 Episode 10）
	if m := reTorrentSxxExx.FindStringSubmatch(work); len(m) >= 3 {
		if s, err := strconv.Atoi(m[1]); err == nil && s >= 0 && s <= maxSeasonNumber {
			info.Season = s
		}
		if ep, ok := parseValidEpisode(m[2]); ok {
			info.Episode = ep
			if len(m) >= 4 && m[3] != "" && info.Version == 0 {
				if v, err := strconv.Atoi(m[3]); err == nil && v >= 2 && v <= 9 {
					info.Version = v
				}
			}
			info.MatchedBy = "sxxexx"
			return info
		}
	}

	// 3. 批量合集范围检测（如 [01-12]、[SP01-04]、全12集）：合集不应误判为单集，返回 Episode = 0
	if isBatchRangeTitle(work) {
		info.IsBatch = true
		if info.IsSpecial && !explicitSeason {
			info.Season = 0
		}
		info.MatchedBy = "batch"
		return info
	}

	// 4. 特别篇带集数（如 SP01、OVA 02、OAD01）统一归入 Specials (Season 0)
	if m := reTorrentSpecialNum.FindStringSubmatch(work); len(m) >= 2 {
		if ep, ok := parseValidEpisode(m[1]); ok {
			info.IsSpecial = true
			info.Season = 0
			info.Episode = ep
			if len(m) >= 3 && m[2] != "" && info.Version == 0 {
				if v, err := strconv.Atoi(m[2]); err == nil && v >= 2 && v <= 9 {
					info.Version = v
				}
			}
			info.MatchedBy = "special"
			return info
		}
	}

	// 如果标题明确标注了 OVA/SP/OAD 且无显式季号，归入 Season 0
	if info.IsSpecial && !explicitSeason {
		info.Season = 0
	}

	// 5. 清洗年份、日期、分辨率、编码及已提取的季号干扰，避免 2024/1080/10bit 误入集数匹配
	cleanForEp := stripEpisodeNoise(work)

	orderedMatchers := []struct {
		name string
		re   *regexp.Regexp
	}{
		{"chinese", reTorrentEpChinese},
		{"ep_prefix", reTorrentEpPrefix},
		{"bracket", reTorrentEpBracket},
		{"dash", reTorrentEpDash},
		{"hash_vol", reTorrentEpHashVol},
		{"version_token", reTorrentEpVerToken},
	}

	for _, matcher := range orderedMatchers {
		// 对每个模式遍历所有匹配，跳过年份/分辨率等误判数字（如 [2024] 后面的 [05]）
		all := matcher.re.FindAllStringSubmatch(cleanForEp, -1)
		for _, m := range all {
			if len(m) < 2 {
				continue
			}
			ep, ok := parseValidEpisode(m[1])
			if !ok {
				continue
			}
			info.Episode = ep
			if len(m) >= 3 && m[2] != "" && info.Version == 0 {
				if v, err := strconv.Atoi(m[2]); err == nil && v >= 2 && v <= 9 {
					info.Version = v
				}
			}
			info.MatchedBy = matcher.name
			return info
		}
	}

	return info
}

func isBatchRangeTitle(work string) bool {
	if matches := reTorrentBatchBracket.FindAllStringSubmatch(work, -1); len(matches) > 0 {
		for _, m := range matches {
			if len(m) < 3 {
				continue
			}
			start, err1 := strconv.Atoi(m[1])
			end, err2 := strconv.Atoi(m[2])
			// 排除年份区间（如 [2023-2024]），仅将合法集数区间识别为合集
			if err1 == nil && err2 == nil && start > 0 && start < 1900 && end > start && end <= maxEpisodeNumber {
				return true
			}
		}
	}
	if reTorrentBatchCN.MatchString(work) {
		return true
	}
	return false
}

func stripEpisodeNoise(s string) string {
	s = reNoiseYearDate.ReplaceAllString(s, " ")
	s = reNoiseCRC32.ReplaceAllString(s, " ")
	s = reNoiseBracketRes.ReplaceAllString(s, " ")
	s = reSeasonNum.ReplaceAllString(s, " ")
	s = reSeasonCN.ReplaceAllString(s, " ")
	s = reSeasonOrd.ReplaceAllString(s, " ")
	s = reTorrentSeason.ReplaceAllString(s, " ")
	return s
}

func parseValidEpisode(raw string) (float32, bool) {
	val, err := strconv.ParseFloat(strings.TrimSpace(raw), 32)
	if err != nil || val <= 0 || val > maxEpisodeNumber {
		return 0, false
	}
	// 排除年份 (1900~2099) 与常见分辨率数字 (360/480/720/1080/1440)
	intVal := int(val)
	if float64(intVal) == val {
		if intVal >= 1900 && intVal <= 2099 {
			return 0, false
		}
		switch intVal {
		case 360, 480, 720, 1080, 1440:
			return 0, false
		}
	}
	return float32(val), true
}

func sanitizeControlRunes(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == 0:
			continue
		case r >= 0x200B && r <= 0x200D:
			continue
		case r >= 0x202A && r <= 0x202E:
			continue
		case r == 0xFEFF:
			continue
		case unicode.IsControl(r):
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i]
		}
		count++
	}
	return s
}
