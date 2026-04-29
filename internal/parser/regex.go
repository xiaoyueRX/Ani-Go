package parser

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

var (
	reAction     = regexp.MustCompile(`^(追番|订阅|取消订阅|退订|查看|列出|搜索|查)`)
	reSeasonNum  = regexp.MustCompile(`第\s*(\d+)\s*季`)
	reSeasonCN   = regexp.MustCompile(`第\s*([一二三四五六七八九十]+)\s*季`)
	reSeasonS    = regexp.MustCompile(`(?i)S(?:eason)?\s*(\d+)`)
	reResolution = regexp.MustCompile(`(?i)(\d{3,4}p|4[kK])`)
	reEpisode    = regexp.MustCompile(`第\s*(\d+)\s*[集話话]`)
	reYear       = regexp.MustCompile(`\((\d{4})\)|（(\d{4})）`)
)

var cnNum = map[rune]int{
	'一': 1, '二': 2, '三': 3, '四': 4, '五': 5,
	'六': 6, '七': 7, '八': 8, '九': 9, '十': 10,
}

type RegexParser struct{}

func NewRegexParser() *RegexParser { return &RegexParser{} }

func (p *RegexParser) Name() string { return "regex" }

func (p *RegexParser) Parse(_ context.Context, input string) (core.ParseResult, error) {
	input = strings.TrimSpace(input)
	result := core.ParseResult{RawInput: input, Season: 1, Confidence: 0.5}

	actionMatch := reAction.FindStringSubmatch(input)
	if len(actionMatch) >= 2 {
		result.Action = mapAction(actionMatch[1])
		input = strings.TrimSpace(input[len(actionMatch[1]):])
	} else {
		result.Action = "subscribe"
		result.Confidence = 0.3
	}

	if m := reSeasonNum.FindStringSubmatch(input); len(m) >= 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			result.Season = n
			result.Confidence += 0.1
		}
		input = strings.Replace(input, m[0], "", 1)
	} else if m := reSeasonCN.FindStringSubmatch(input); len(m) >= 2 {
		result.Season = parseCNNumber(m[1])
		result.Confidence += 0.1
		input = strings.Replace(input, m[0], "", 1)
	} else if m := reSeasonS.FindStringSubmatch(input); len(m) >= 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			result.Season = n
			result.Confidence += 0.1
		}
		input = strings.Replace(input, m[0], "", 1)
	}

	if m := reResolution.FindStringSubmatch(input); len(m) >= 2 {
		result.Resolution = strings.ToLower(m[1])
		result.Confidence += 0.05
		input = strings.Replace(input, m[0], "", 1)
	}

	if m := reEpisode.FindStringSubmatch(input); m != nil {
		input = strings.Replace(input, m[0], "", 1)
	}

	if m := reYear.FindStringSubmatch(input); m != nil {
		input = strings.Replace(input, m[0], "", 1)
	}

	result.Title = cleanTitle(input)
	if result.Title != "" {
		result.Confidence += 0.2
	}

	if result.Title == "" {
		result.Title = input
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
		}
	}
	if result == 0 {
		result = 1
	}
	return result
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimFunc(s, func(r rune) bool {
		return r == ' ' || r == '，' || r == ',' || r == '。' || r == '.'
	})
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	reRes := regexp.MustCompile(`(?i)\b(2160p|1440p|1080p|720p|480p|4[kK])\b`)
	s = reRes.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return s
}
