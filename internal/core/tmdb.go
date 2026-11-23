package core

import "strings"

// ParseTMDBID 解析 TMDB ID 的媒体类型和纯数字/字符串标识
// 支持 "tv/123"、"movie/456" 或裸 ID "123"（默认归为 tv）
func ParseTMDBID(raw string) (mediaType string, cleanID string) {
	s := strings.Trim(strings.TrimSpace(raw), "/")
	if s == "" {
		return "", ""
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "movie/") {
		id := strings.Trim(s[len("movie/"):], "/")
		if id == "" {
			return "", ""
		}
		return "movie", id
	}
	if strings.HasPrefix(lower, "tv/") {
		id := strings.Trim(s[len("tv/"):], "/")
		if id == "" {
			return "", ""
		}
		return "tv", id
	}
	return "tv", s
}

// TMDBMediaType 返回本地分类明确指向的 TMDB 命名空间，未知分类留空。
func TMDBMediaType(animeType string) string {
	switch strings.ToLower(strings.TrimSpace(animeType)) {
	case "movie", "film", "劇場版", "剧场版":
		return "movie"
	case "tv", "series", "animation", "scripted", "miniseries":
		return "tv"
	default:
		return ""
	}
}
