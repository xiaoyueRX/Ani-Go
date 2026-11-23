package scheduler

import (
	"context"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

func TestReleaseAliasMatchingForFeedAndHistory(t *testing.T) {
	sub := database.Subscription{TitleCN: "中文片名", TitleJP: "日本語名", TitleEN: "Calendar Show", Season: 1}
	for _, tc := range []struct {
		title string
		sub   database.Subscription
		want  bool
	}{
		{"[组] 中文片名 / 日本語名 - 01 [1080p]", sub, true},
		{"[组] 其他译名 | 日本語名 - 02", sub, true},
		{"[组] 中文片名／日本語名 - 03", sub, true},
		{"[组] 中文片名/日本語名 - 04", sub, true},
		{"[Group] [中文片名] [05] [1080p]", sub, true},
		{"[Group] Calendar Show - 06", sub, true},
		{"[Group] Calendar Show Sequel - 07", sub, false},
		{"[Group] Fate/Zero - 08", database.Subscription{TitleEN: "Fate"}, false},
		{"[Group] Fate/Zero - 09", database.Subscription{TitleEN: "Fate/Zero"}, true},
		{"[Group] Calendar Show S02E10", sub, false},
	} {
		t.Run(tc.title, func(t *testing.T) {
			item := core.TorrentItem{Title: tc.title, SourceName: "Nyaa"}
			if match, ambiguous := matchFeedSubscription(item, []database.Subscription{tc.sub}); (match != nil) != tc.want || ambiguous {
				t.Errorf("RSS 归属为 %+v，存在歧义 %v，期望匹配 %v", match, ambiguous, tc.want)
			}
			if got := matchesHistoryRelease(item, tc.sub, core.Filter{}); got != tc.want {
				t.Errorf("历史匹配 %v，期望 %v", got, tc.want)
			}
		})
	}
}

func TestMikanHistoryRejectsDifferentResourceID(t *testing.T) {
	sub := database.Subscription{TitleCN: "同名作品", MikanID: "123"}
	item := core.TorrentItem{Title: "同名作品 - 01", SourceName: "Mikan", BangumiID: "456"}
	if matchesHistoryRelease(item, sub, core.Filter{}) {
		t.Fatal("其他蜜柑条目被当成本订阅的历史资源")
	}
}

type catalogFixture struct {
	results  []core.TorrentItem
	subjects map[string]string
}

func (f catalogFixture) SearchAnime(context.Context, string) ([]core.TorrentItem, error) {
	return f.results, nil
}

func (f catalogFixture) GetAnimeInfo(_ context.Context, id string) (*source.MikanAnimeInfo, error) {
	return &source.MikanAnimeInfo{SubjectID: f.subjects[id]}, nil
}

func TestMikanCatalogChecksBangumiSubject(t *testing.T) {
	sub := database.Subscription{TitleCN: "海港物语", MetadataProvider: "BGM.tv", MetadataID: "500"}
	first := core.TorrentItem{Title: sub.TitleCN, BangumiID: "10"}
	second := core.TorrentItem{Title: sub.TitleCN, BangumiID: "20"}
	for _, tc := range []struct {
		name    string
		fixture catalogFixture
		want    string
	}{
		{"同名的其他条目", catalogFixture{[]core.TorrentItem{first}, map[string]string{"10": "999"}}, ""},
		{"缺少条目链接", catalogFixture{[]core.TorrentItem{first}, nil}, ""},
		{"同名时由详情区分", catalogFixture{[]core.TorrentItem{first, second}, map[string]string{"10": "999", "20": "500"}}, "20"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := lookupMikanID(t.Context(), tc.fixture, sub); got != tc.want {
				t.Errorf("关联到蜜柑 %q，期望 %q", got, tc.want)
			}
		})
	}
}

func TestCollectionRebindsLegacyIdentityAndPreservesCustomFeeds(t *testing.T) {
	for _, tc := range []struct{ name, url, wantURL string }{
		{"旧版自动生成", "https://mikan.example/RSS/Bangumi?bangumiId=42&subgroupid=7", ""},
		{"用户维护的地址", "https://custom.example/my.rss", "https://custom.example/my.rss"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := pipelineDB(t)
			sub := database.Subscription{TitleCN: "海港物语", BangumiID: "42", SourceName: "Mikan", RSSURL: tc.url}
			savePipelineRow(t, db, &sub)
			collection := core.Anime{TitleCN: sub.TitleCN, ID: "42", Provider: "BGM.tv"}
			if _, err := (&Scheduler{}).autoSubscribe(t.Context(), collection); err != nil {
				t.Fatal(err)
			}
			var got []database.Subscription
			if err := db.Find(&got).Error; err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].ID != sub.ID || got[0].MetadataID != "42" || got[0].MikanID != "" || got[0].BangumiID != "" || got[0].RSSURL != tc.wantURL {
				t.Fatalf("收藏未保留正确的订阅、元数据或 RSS 地址：%+v", got)
			}
		})
	}
}

func TestBilingualReleaseKeepsAmbiguousSeasonsSeparate(t *testing.T) {
	subs := []database.Subscription{{TitleCN: "海港", Season: 1}, {TitleEN: "Harbor Story", Season: 2}}
	item := core.TorrentItem{Title: "[Group] 海港 / Harbor Story - 01", SourceName: "Nyaa"}
	if match, ambiguous := matchFeedSubscription(item, subs); match != nil || !ambiguous {
		t.Fatalf("双语标题不能决定季度，实际匹配 %+v，歧义 %v", match, ambiguous)
	}
}
