package scheduler

import (
	"context"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

func TestTMDBTitleFallbackUsesExplicitSeason(t *testing.T) {
	db := pipelineDB(t)
	seasonOne := database.Subscription{TitleCN: "海港", MetadataProvider: "TMDB", MetadataID: "765432", Season: 1}
	savePipelineRow(t, db, &seasonOne)
	s := &Scheduler{metadataProvider: &feedMetadata{}}
	release := core.TorrentItem{Title: "[字幕组] 海港 第二季 - 01 [1080p]"}
	id, err := createSubscriptionFromTitle(t.Context(), s, release, "海港 第二季")
	if err != nil || id == 0 {
		t.Fatalf("第二季未创建：%d，%v", id, err)
	}
	var sub database.Subscription
	if err := db.First(&sub, id).Error; err != nil || sub.Season != 2 || sub.MetadataID != "765432" || sub.TitleCN != "海港" {
		t.Fatalf("预期按作品和第二季创建，实际 %+v，错误 %v", sub, err)
	}
	if id, err := createSubscriptionFromTitle(t.Context(), s, release, "海港 第二季"); err != nil || id != 0 {
		t.Fatalf("同一季重复创建：%d，%v", id, err)
	}
}

func TestTMDBTitleFallbackLeavesLegacySeasonUntouched(t *testing.T) {
	db := pipelineDB(t)
	first := database.Subscription{TitleCN: "海港", BangumiID: "765432", Season: 1}
	savePipelineRow(t, db, &first)
	s := &Scheduler{metadataProvider: &feedMetadata{}}
	id, err := createSubscriptionFromTitle(t.Context(), s, core.TorrentItem{Title: "[字幕组] 海港 S02E01"}, "海港")
	if err != nil || id == 0 {
		t.Fatalf("第二季未创建：%d，%v", id, err)
	}
	if err := db.First(&first, first.ID).Error; err != nil || first.MetadataID != "" {
		t.Fatalf("旧第一季标识被改写：%+v，错误 %v", first, err)
	}
}

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

func TestAutoSubscribeDistinguishesSameTMDBNumberByMediaType(t *testing.T) {
	db := pipelineDB(t)
	s := Scheduler{}
	for i, anime := range []core.Anime{
		{ID: "123", TMDBID: "tv/123", Type: "TV", Season: 1},
		{ID: "123", TMDBID: "movie/123", Type: "Movie", Season: 1},
		{ID: "movie/123", Type: "Movie", Season: 5},
		{ID: "tv/123", Type: "TV", Season: 1},
		{ID: "123", TMDBID: "tv/123", Type: "TV", Season: 2},
	} {
		anime.Provider = "TMDB"
		anime.TitleCN = "同名作品"
		id, err := s.autoSubscribe(t.Context(), anime)
		created := i == 0 || i == 1 || i == 4
		if err != nil || (id != 0) != created {
			t.Errorf("第 %d 项创建 ID %d，错误 %v", i, id, err)
		}
	}
	var count int64
	if err := db.Model(&database.Subscription{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("实际 %d 项，错误 %v", count, err)
	}
}

func TestAutoSubscribeTMDBRebindChecksLegacyMovieEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, tmdbID, animeType string
		season                  int
		reuse                   bool
	}{
		{"电影与历史季号无关", "", "", 3, true},
		{"同名同号电视剧不应改为电影", "tv/123", "", 1, false},
		{"带电视剧分类的旧订阅", "", "TV", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := pipelineDB(t)
			old := database.Subscription{TitleCN: "同名", BangumiID: "123", TMDBID: tc.tmdbID, AnimeType: tc.animeType, Season: tc.season}
			savePipelineRow(t, db, &old)
			id, err := (&Scheduler{}).autoSubscribe(t.Context(), core.Anime{ID: "123", TMDBID: "movie/123", Type: "Movie", Provider: "TMDB", TitleCN: "同名", Season: 1})
			if err != nil || (id == 0) != tc.reuse {
				t.Fatalf("实际新增 ID %d，期望沿用旧记录 %t，错误 %v", id, tc.reuse, err)
			}
			if err := db.First(&old, old.ID).Error; err != nil {
				t.Fatal(err)
			}
			if tc.reuse && old.TMDBID != "movie/123" || !tc.reuse && (old.TMDBID != tc.tmdbID || old.MetadataID != "") {
				t.Fatalf("旧记录的作品标识错误：%+v", old)
			}
		})
	}
}
