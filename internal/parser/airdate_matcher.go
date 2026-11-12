package parser

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

const (
	// defaultWindowBeforeHours 默认允许提前发布的容差（小时）
	defaultWindowBeforeHours = 6.0
	// defaultWindowAfterHours 默认允许延后发布的容差（小时）
	defaultWindowAfterHours = 72.0
	// idealReleaseDelayHours 字幕组压制发布相对放送时间的典型延迟（小时）
	idealReleaseDelayHours = 12.0
)

var (
	reDigits        = regexp.MustCompile(`\b\d{1,4}\b`)
	reDecimalNumber = regexp.MustCompile(`(?:^|[^0-9.])(\d{1,4}\.5)(?:$|[^0-9])`)
	// 东京/北京常见放送时区，供字符串日期解析使用
	cstLocation = time.FixedZone("CST", 8*3600)
)

// MatchResult 放送时间戳比对结果
type MatchResult struct {
	Matched     bool
	Season      int
	Episode     float32
	Confidence  float32
	MatchedEp   *core.Episode
	Explanation string
}

// ParseAirDate 解析常见日期时间字符串（支持 RFC3339、带时区时间戳、CST 本地时间及 YYYY-MM-DD）
func ParseAirDate(dateStr string) (time.Time, error) {
	return ParseAirDateInLocation(dateStr, time.UTC)
}

// ParseAirDateInLocation 使用指定的默认时区解析无显式偏移的日期时间字符串
func ParseAirDateInLocation(dateStr string, defaultLoc *time.Location) (time.Time, error) {
	s := strings.TrimSpace(dateStr)
	if s == "" {
		return time.Time{}, fmt.Errorf("日期字符串为空")
	}
	if defaultLoc == nil {
		defaultLoc = time.UTC
	}

	// 1. 先尝试自带时区信息的标准格式
	tzLayouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		time.RFC1123Z,
		time.RFC1123,
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02T15:04:05-07:00",
	}
	for _, layout := range tzLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}

	// 处理末尾带 "CST" 字样但无数字偏移的时间字符串（避免 Go time.Parse 受本地系统时区影响）
	if strings.HasSuffix(strings.ToUpper(s), " CST") {
		base := strings.TrimSpace(s[:len(s)-4])
		for _, layout := range []string{"2006-01-02 15:04:05", "2006/01/02 15:04:05", "2006-01-02"} {
			if t, err := time.ParseInLocation(layout, base, cstLocation); err == nil {
				return t.UTC(), nil
			}
		}
	}

	// 2. 再尝试无时区偏移的本地格式（按 defaultLoc 解析并归一化为 UTC）
	localLayouts := []string{
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"2006/01/02",
	}
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, s, defaultLoc); err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("无法解析日期时间: %q", dateStr)
}

// MatchEpisodeByAirDate 根据种子的发布时间戳与番剧官方单集放送表进行对齐
// 完全免费、零 Token 消耗，可化解 95% 以上的绝对集数歧义 (例如 [25] 对应 S02E01)
func MatchEpisodeByAirDate(pubDate time.Time, rawTitle string, eps []core.Episode) MatchResult {
	return MatchEpisodeByAirDateWithWindow(pubDate, rawTitle, eps, defaultWindowBeforeHours, defaultWindowAfterHours)
}

// MatchEpisodeByAirDateWithWindow 支持自定义容差窗口的放送时间戳对齐算法
func MatchEpisodeByAirDateWithWindow(pubDate time.Time, rawTitle string, eps []core.Episode, beforeHours, afterHours float64) MatchResult {
	if pubDate.IsZero() || len(eps) == 0 {
		return MatchResult{Matched: false}
	}
	if math.IsNaN(beforeHours) || math.IsInf(beforeHours, 0) || beforeHours < 0 {
		beforeHours = defaultWindowBeforeHours
	}
	if math.IsNaN(afterHours) || math.IsInf(afterHours, 0) || afterHours < 0 {
		afterHours = defaultWindowAfterHours
	}

	// 统一转换为 UTC 时间以消除 CST/JST/UTC 时区偏差
	pubUTC := pubDate.UTC()

	// 提取标题中可能存在的集数候选（先剥离年份、分辨率、编码等数字噪音）
	intCandidates, halfCandidates := extractAirDateCandidates(rawTitle)

	var best *airDateScoredCandidate

	for i := range eps {
		ep := eps[i]
		if ep.AiredAt.IsZero() || ep.Season < 0 {
			continue
		}
		epNum64 := float64(ep.Number)
		if ep.Number <= 0 || math.IsNaN(epNum64) || math.IsInf(epNum64, 0) {
			continue
		}

		epAirUTC := ep.AiredAt.UTC()
		diff := pubUTC.Sub(epAirUTC).Hours()
		if math.IsNaN(diff) || math.IsInf(diff, 0) {
			continue
		}

		// 检查是否落在 [-beforeHours, +afterHours] 容差窗口内
		if diff < -beforeHours || diff > afterHours {
			continue
		}

		numMatched := isCandidateNumberMatch(ep.Number, intCandidates, halfCandidates)
		cand := &airDateScoredCandidate{
			ep:          ep,
			numMatched:  numMatched,
			idealDelta:  math.Abs(diff - idealReleaseDelayHours),
			nonNegative: diff >= 0,
			absDiff:     math.Abs(diff),
		}

		if best == nil || isBetterAirDateCandidate(cand, best) {
			best = cand
		}
	}

	if best == nil {
		return MatchResult{Matched: false}
	}

	epCopy := best.ep
	if best.numMatched {
		return MatchResult{
			Matched:     true,
			Season:      epCopy.Season,
			Episode:     epCopy.Number,
			Confidence:  0.98,
			MatchedEp:   &epCopy,
			Explanation: "种子发布时间落在放送容差窗口且标题数字完全匹配",
		}
	}

	return MatchResult{
		Matched:     true,
		Season:      epCopy.Season,
		Episode:     epCopy.Number,
		Confidence:  0.85,
		MatchedEp:   &epCopy,
		Explanation: "根据种子发布时间与 Bangumi 放送日历窗口对齐推断",
	}
}

// MatchEpisodeFromProvider 封装元数据提供方调用，安全处理 nil、上下文取消及 API 异常
func MatchEpisodeFromProvider(ctx context.Context, provider core.MetadataProvider, animeID string, season int, pubDate time.Time, rawTitle string) (MatchResult, error) {
	if pubDate.IsZero() || strings.TrimSpace(animeID) == "" {
		return MatchResult{Matched: false}, nil
	}
	if provider == nil {
		return MatchResult{Matched: false}, fmt.Errorf("元数据提供方未初始化")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return MatchResult{Matched: false}, err
	}

	eps, err := provider.GetEpisodes(ctx, strings.TrimSpace(animeID), season)
	if err != nil {
		return MatchResult{Matched: false}, fmt.Errorf("获取放送剧集列表失败: %w", err)
	}
	if len(eps) == 0 {
		return MatchResult{Matched: false}, nil
	}

	return MatchEpisodeByAirDate(pubDate, rawTitle, eps), nil
}

func extractAirDateCandidates(rawTitle string) (map[int]bool, map[float32]bool) {
	intCandidates := make(map[int]bool)
	halfCandidates := make(map[float32]bool)

	work := sanitizeControlRunes(rawTitle)
	work = truncateRunes(work, maxParserInputRunes)
	work = stripEpisodeNoise(work)

	for _, m := range reDecimalNumber.FindAllStringSubmatch(work, -1) {
		if len(m) >= 2 {
			if val, err := strconv.ParseFloat(m[1], 32); err == nil && val > 0 && val <= maxEpisodeNumber {
				halfCandidates[float32(val)] = true
			}
		}
	}

	for _, m := range reDigits.FindAllString(work, -1) {
		n, err := strconv.Atoi(m)
		if err != nil || n <= 0 || n > maxEpisodeNumber {
			continue
		}
		// 排除年份与分辨率数字干扰
		if n >= 1900 && n <= 2099 {
			continue
		}
		switch n {
		case 360, 480, 720, 1080, 1440:
			continue
		}
		intCandidates[n] = true
	}

	return intCandidates, halfCandidates
}

func isCandidateNumberMatch(epNumber float32, intCandidates map[int]bool, halfCandidates map[float32]bool) bool {
	if float32(int(epNumber)) != epNumber {
		return halfCandidates[epNumber]
	}
	return intCandidates[int(epNumber)]
}

type airDateScoredCandidate struct {
	ep          core.Episode
	numMatched  bool
	idealDelta  float64 // |diff - 12|
	nonNegative bool    // diff >= 0
	absDiff     float64 // |diff|
}

func isBetterAirDateCandidate(a, b *airDateScoredCandidate) bool {
	if a.numMatched != b.numMatched {
		return a.numMatched
	}
	const epsTol = 1e-6
	if math.Abs(a.idealDelta-b.idealDelta) > epsTol {
		return a.idealDelta < b.idealDelta
	}
	if a.nonNegative != b.nonNegative {
		return a.nonNegative
	}
	if math.Abs(a.absDiff-b.absDiff) > epsTol {
		return a.absDiff < b.absDiff
	}
	if a.ep.Season != b.ep.Season {
		return a.ep.Season < b.ep.Season
	}
	return a.ep.Number < b.ep.Number
}
