package parser

import (
	"path/filepath"
	"strings"
)

// DirectorySeason 检查相对路径父目录中的季标记。
// 遇到合季目录、单段多季或各级父目录季号冲突时返回 ambiguous=true。
func DirectorySeason(rel string) (season int, explicit bool, ambiguous bool) {
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." || dir == "" || dir == "/" {
		return 0, false, false
	}
	for _, segment := range strings.Split(dir, "/") {
		if segment == "" || segment == "." {
			continue
		}
		segSeason, segExplicit, segAmbiguous := unambiguousSeason(segment, 0)
		if segAmbiguous {
			return 0, false, true
		}
		if !segExplicit {
			continue
		}
		if explicit && segSeason != season {
			return 0, false, true
		}
		season = segSeason
		explicit = true
	}
	return season, explicit, false
}
