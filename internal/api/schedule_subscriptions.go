package api

import (
	"slices"
	"strconv"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

// Zero records a missing or ambiguous match. Neither can identify a subscription.
func addScheduleMatch(index map[string]uint, key string, id uint) {
	if key == "" {
		return
	}
	if previous, exists := index[key]; exists && previous != id {
		id = 0
	}
	index[key] = id
}

// A provider can cache a season's schedule. Annotate a copy for the current request.
func scheduleSubscriptions(schedule []source.WeekDayItem, subs []database.Subscription) ([]source.WeekDayItem, map[string]uint) {
	bgm, mikan, titles := map[string]uint{}, map[string]uint{}, map[string]uint{}
	result := map[string]uint{}
	for _, sub := range subs {
		addScheduleMatch(bgm, sub.BGMSubjectID(), sub.ID)
		addScheduleMatch(mikan, sub.MikanID, sub.ID)
		for _, name := range []string{sub.TitleCN, sub.TitleJP, sub.TitleEN} {
			key := core.NormalizeTitle(strings.TrimSpace(name))
			if key != "" {
				addScheduleMatch(titles, key, sub.ID)
				result[name] = 0
			}
		}
	}
	for name := range result {
		result[name] = titles[core.NormalizeTitle(strings.TrimSpace(name))]
	}

	days := slices.Clone(schedule)
	for d := range days {
		days[d].Items = slices.Clone(days[d].Items)
		for i := range days[d].Items {
			item := &days[d].Items[i]
			var id uint
			switch strings.ToLower(item.SourceName) {
			case "mikan":
				id = mikan[item.BangumiID]
			case "bgm.tv":
				id = bgm[item.BangumiID]
			}
			if id == 0 {
				id = titles[core.NormalizeTitle(strings.TrimSpace(item.Title))]
			}
			item.InfoHash = ""
			if id != 0 {
				item.InfoHash = strconv.FormatUint(uint64(id), 10)
			}
			addScheduleMatch(result, item.BangumiID, id)
		}
	}
	for key, id := range result {
		if id == 0 {
			delete(result, key)
		}
	}
	return days, result
}
