package api

import (
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
	"gorm.io/gorm"
)

func TestScheduleKeepsCachedDataIntactAndUsesUnambiguousIdentities(t *testing.T) {
	subs := []database.Subscription{
		{Model: gorm.Model{ID: 3}, TitleCN: "Harbor Story", TitleEN: "First Harbor", Season: 1, MikanID: "12"},
		{Model: gorm.Model{ID: 4}, TitleCN: "Harbor Story", Season: 2, MetadataID: "12", MetadataProvider: "BGM.tv"},
	}
	schedule := []source.WeekDayItem{{Items: []core.TorrentItem{
		{Title: "Harbor Story", SourceName: "Yuc"},
		{Title: "First Harbor", SourceName: "Yuc"},
		{Title: "Harbor Story", SourceName: "Mikan", BangumiID: "12"},
		{Title: "Harbor Story", SourceName: "BGM.tv", BangumiID: "12"},
	}}}
	view, lookup := scheduleSubscriptions(schedule, subs)
	for i, want := range []string{"", "3", "3", "4"} {
		if got := view[0].Items[i].InfoHash; got != want {
			t.Errorf("第 %d 项订阅 ID=%q，期望 %q", i, got, want)
		}
	}
	if _, ok := lookup["Harbor Story"]; ok {
		t.Error("同名多季回退到了不确定的订阅")
	}
	if _, ok := lookup["12"]; ok {
		t.Error("两个站点的相同数字不应成为独立查找键")
	}
	for i, item := range schedule[0].Items {
		if item.InfoHash != "" {
			t.Errorf("第 %d 项缓存被写入了订阅 %s", i, item.InfoHash)
		}
	}
}
