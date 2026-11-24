package parser

import (
	"regexp"
	"strconv"
)

// DeclaredEpisodeRange 只返回发行标题明确标注的正片起止集号。全 N 集只表明数量，可能采用全剧连续编号。
func DeclaredEpisodeRange(title string) (int, int) {
	title = truncateRunes(sanitizeControlRunes(title), maxParserInputRunes)
	start, end := 0, 0
	for _, pattern := range []*regexp.Regexp{reTorrentBatchBracket, reTorrentBatchCN} {
		for _, match := range pattern.FindAllStringSubmatch(title, -1) {
			if match[1] == "" || reTorrentSpecialNum.MatchString(match[0]) {
				continue
			}
			first, _ := strconv.Atoi(match[1])
			last, _ := strconv.Atoi(match[2])
			if first <= 0 || first >= last || last > maxEpisodeNumber {
				continue
			}
			if start != 0 && (first != start || last != end) {
				return 0, 0
			}
			start, end = first, last
		}
	}
	return start, end
}
