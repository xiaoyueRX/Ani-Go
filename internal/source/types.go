package source

import "github.com/xiaoyueRX/Ani-Go/internal/core"

// WeekDayItem 表示某一天播放的番剧列表
type WeekDayItem struct {
	DayOfWeek int               `json:"day_of_week"`
	Label     string            `json:"label"`
	Items     []core.TorrentItem `json:"items"`
}
