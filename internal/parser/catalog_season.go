package parser

import (
	"regexp"
	"strconv"
)

const catalogCNNumber = "[一二三四五六七八九十0-9]{1,4}"
const catalogSeasonJoin = `\s*(?:[-~～–—/、,，+&至到和及]|and|to)\s*`

// 有些合季标题只写一次“季”，例如“第二至第三季”或“Season 2–4”。
var combinedCatalogSeasons = regexp.MustCompile(`(?i)(?:第\s*` + catalogCNNumber + `\s*季?` + catalogSeasonJoin + `(?:第\s*)?` + catalogCNNumber + `\s*季|(?:^|[^a-z0-9])(?:seasons?|s)\s*[0-9]{1,2}` + catalogSeasonJoin + `[0-9]{1,2}(?:$|[^a-z0-9]))`)
var singleCNSeason = regexp.MustCompile(`^(?:[一二三四五六七八九]|[一二三四五六七八九]?十[一二三四五六七八九]?)$`)

// CatalogSeason 只从作品标题中一致的明确季号推断新增订阅的季。模糊或合季标题沿用第一季默认值。
func CatalogSeason(title string) int {
	title = sanitizeControlRunes(truncateRunes(title, maxParserInputRunes))
	if combinedCatalogSeasons.MatchString(title) {
		return 1
	}
	season := 0
	for _, pattern := range []*regexp.Regexp{reSeasonNum, reSeasonCN, reTorrentSeason, reSeasonOrd, reTorrentSxxExx} {
		for remaining := title; ; {
			match := pattern.FindStringSubmatchIndex(remaining)
			if match == nil {
				break
			}
			number := remaining[match[2]:match[3]]
			value, _ := strconv.Atoi(number)
			if pattern == reSeasonCN {
				if !singleCNSeason.MatchString(number) {
					return 1
				}
				value = parseCNNumber(number)
			}
			if value < 1 || value > maxSeasonNumber || (season != 0 && season != value) {
				return 1
			}
			season = value
			// 保留匹配末尾的分隔符，使 S02/S03 的后一个标记也能被识别。
			remaining = remaining[match[3]:]
		}
	}
	return max(1, season)
}
