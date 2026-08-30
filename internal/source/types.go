package source

import "github.com/xiaoyueRX/Ani-Go/internal/core"

// WeekDayItem 表示某一天播放的番剧列表
type WeekDayItem struct {
	DayOfWeek int               `json:"day_of_week"`
	Label     string            `json:"label"`
	Items     []core.TorrentItem `json:"items"`
}

// SeasonAnimeInfo 包含 yuc.wiki 抓取的更多元数据
type SeasonAnimeInfo struct {
	Title         string `json:"title"`
	CoverURL      string `json:"cover_url"`
	TotalEpisodes int    `json:"total_episodes"`
	IsFinished    bool   `json:"is_finished"`
	BroadcastTime string `json:"broadcast_time"`
}
