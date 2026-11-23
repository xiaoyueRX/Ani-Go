package core

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// 带边界匹配发行附加内容标记，保留 OPUS、Previewing 等普通词。
var movieExtraWord = regexp.MustCompile(`(?:^| )(?:trailers?|teasers?|preview|sample|promo|pv|featurettes?|extras?|bonuses|bonus|making of|behind the scenes|interviews?|ncop|nced|op|ed|menus?)[0-9]*(?: |$)|预告|預告|予告|特報|特报|花絮|特典|メイキング`)

// MovieExtra 检查源文件、相对目录或发布名。先排除片名本身，避免把 The Menu 等正片送去复核。
func MovieExtra(name string, titles ...string) bool {
	words := func(s string) string {
		return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		}), " ")
	}
	var aliases []string
	for _, title := range titles {
		if title = words(title); title != "" && !slices.Contains(aliases, title) {
			aliases = append(aliases, title)
		}
	}
	slices.SortFunc(aliases, func(a, b string) int { return len(b) - len(a) })
	for _, part := range strings.Split(strings.ReplaceAll(name, "\\", "/"), "/") {
		release := " " + words(part) + " "
		for _, title := range aliases {
			release = strings.Replace(release, " "+title+" ", " ", 1)
		}
		if movieExtraWord.MatchString(release) {
			return true
		}
	}
	return false
}
