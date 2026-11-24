package scheduler

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
)

// 只检查独立的片头、片尾和菜单标记，避免把 OPUS 等编码信息当作花絮。
var supplementalVideoName = regexp.MustCompile(`(?i)(^|[ ._\[\]()-])(NCOP|NCED|OP|ED|MENU)[ ._-]*[0-9]{0,3}(?:[ ._-]*(?:\[[^\]]*\]|\([^)]*\)))*[ ._-]*$`)

func batchEpisodeReviewReason(rel string, sub database.Subscription, season int, number float32) string {
	name := filepath.Base(rel)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if supplementalVideoName.MatchString(stem) {
		return fmt.Sprintf("请核对片头、片尾或菜单的归属：%s", rel)
	}
	subSeason := max(1, sub.Season)
	dirSeason, dirExplicit, dirAmbiguous := parser.DirectorySeason(rel)
	if dirAmbiguous {
		return fmt.Sprintf("目录包含多个或冲突的季标记，请核对归属：%s", rel)
	}
	if dirExplicit {
		if dirSeason == 0 {
			if season != 0 || number <= 0 {
				return fmt.Sprintf("目录标记为特别篇（第 0 季），缺少明确的特别篇集号：%s", rel)
			}
		} else {
			if dirSeason != subSeason {
				return fmt.Sprintf("目录标记为第 %d 季，订阅为第 %d 季：%s", dirSeason, subSeason, rel)
			}
			if season > 0 && season != dirSeason {
				return fmt.Sprintf("目录标记为第 %d 季，文件标记为第 %d 季：%s", dirSeason, season, rel)
			}
		}
	}
	if season == 0 && number > 0 {
		return ""
	}
	for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/") {
		switch strings.ToLower(part) {
		case "extra", "extras", "featurettes", "bonus", "bonuses", "特典", "花絮":
			return fmt.Sprintf("附加视频缺少明确的特别篇集号：%s", rel)
		}
	}
	if number > 0 && season > 0 {
		if season != subSeason {
			return fmt.Sprintf("文件标记为第 %d 季，订阅为第 %d 季：%s", season, subSeason, rel)
		}
		if sub.TotalEpisodes > 0 && number > float32(sub.TotalEpisodes) {
			return fmt.Sprintf("文件集号 %g 超过本季已知的 %d 集，请核对本季集号：%s", number, sub.TotalEpisodes, rel)
		}
	}
	return ""
}

// 只有原始编号和已解析的季内编号都不在发行范围内时才产生冲突，保留可验证的跨季编号映射。
func batchRangeReviewReason(first, last, season int, number, sourceNumber float32) string {
	if first == 0 || season <= 0 || number <= 0 {
		return ""
	}
	within := func(n float32) bool { return n >= float32(first) && n <= float32(last) }
	if within(number) || within(sourceNumber) {
		return ""
	}
	return fmt.Sprintf("文件集号 %g 与合集声明的 %d–%d 集不一致，请核对季内集号", number, first, last)
}
