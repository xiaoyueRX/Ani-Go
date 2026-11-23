package scheduler

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// 只检查独立的片头、片尾和菜单标记，避免把 OPUS 等编码信息当作花絮。
var supplementalVideoName = regexp.MustCompile(`(?i)(^|[ ._\[\]()-])(NCOP|NCED|OP|ED|MENU)[ ._-]*[0-9]{0,3}(?:[ ._-]*(?:\[[^\]]*\]|\([^)]*\)))*[ ._-]*$`)

func batchEpisodeReviewReason(rel string, sub database.Subscription, season int, number float32) string {
	name := filepath.Base(rel)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if supplementalVideoName.MatchString(stem) {
		return fmt.Sprintf("请核对片头、片尾或菜单的归属：%s", rel)
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
		if season != max(1, sub.Season) {
			return fmt.Sprintf("文件标记为第 %d 季，订阅为第 %d 季：%s", season, max(1, sub.Season), rel)
		}
		if sub.TotalEpisodes > 0 && number > float32(sub.TotalEpisodes) {
			return fmt.Sprintf("文件集号 %g 超过本季已知的 %d 集，请核对本季集号：%s", number, sub.TotalEpisodes, rel)
		}
	}
	return ""
}
