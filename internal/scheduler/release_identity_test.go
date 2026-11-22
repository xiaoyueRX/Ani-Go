package scheduler

import (
	"context"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
	"testing"
	"time"
)

func subscriptionTestDB(t *testing.T) {
	t.Helper()
	old := database.DB
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	db, err := database.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); database.DB = old })
}

type releaseFixture struct {
	core.Source
	items   []core.TorrentItem
	filter  core.Filter
	mikanID string
}

func (r *releaseFixture) FetchHistory(_ context.Context, id string, filter core.Filter) ([]core.TorrentItem, error) {
	r.filter, r.mikanID = filter, id
	return r.items, nil
}
func (r *releaseFixture) FetchRSS(context.Context, string) ([]core.TorrentItem, error) {
	return r.items, nil
}

func TestSupplementChecksSeriesSeasonAndRulesBeforeDownload(t *testing.T) {
	subscriptionTestDB(t)
	sub := database.Subscription{TitleCN: "中文名", TitleEN: "Work A", Season: 2, SubgroupName: "Group", FilterJSON: `{"exclude_keywords":["720p"]}`}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	src := &releaseFixture{}
	for _, title := range []string{"[Group] Work B S02E01 [1080p]", "[Group] Work A Sequel S02E01 [1080p]", "[Group] Work A S01E01 [1080p]", "[Group] Work A S02E01 [720p]", "[Group] Work A S02E01 [1080p]"} {
		src.items = append(src.items, core.TorrentItem{Title: title, URL: title})
	}
	(&Scheduler{source: src, downloader: &mockSchedDownloader{}}).supplementOne(context.Background(), sub)
	var eps []database.Episode
	if err := database.DB.Find(&eps).Error; err != nil {
		t.Fatal(err)
	}
	if len(eps) != 1 || eps[0].Title != src.items[4].Title || eps[0].Season != 2 || eps[0].Number != 1 || src.filter.SearchTitle != "Work A" {
		t.Fatalf("补全结果错误：%+v, %+v", eps, src.filter)
	}
}

func TestPersonalRSSUsesAliasesAndPreviewRules(t *testing.T) {
	subscriptionTestDB(t)
	if err := source.SetCustomRegexPatterns([]string{`CODE-(\d+)`}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { source.SetCustomRegexPatterns(nil) })
	sub := database.Subscription{TitleCN: "中文标题", TitleEN: "Sample Anime", TitleJP: "日本語名"}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	src := &releaseFixture{items: []core.TorrentItem{
		{Title: "[Group] Sample Anime CODE-007", URL: "one"},
		{Title: "[Group] 日本語名 CODE-008", URL: "two"},
	}}
	s := &Scheduler{source: src, downloader: &mockSchedDownloader{}, mikanRSSURL: "test-feed"}
	s.pollRSS(context.Background())
	var eps []database.Episode
	database.DB.Order("id").Find(&eps)
	if len(eps) != 2 {
		t.Fatalf("按别名订阅只入库 %d 个条目", len(eps))
	}
	for i, ep := range eps {
		preview := source.ParseMikanTitle(src.items[i].Title)
		if ep.Number != float32(7+i) || ep.Number != preview.Episode {
			t.Fatalf("解析结果不一致 %+v / %+v", ep, preview)
		}
	}
}

func TestCompletionRebindsURLPlaceholder(t *testing.T) {
	subscriptionTestDB(t)
	createEpisodeRecordWithParsed(1, core.TorrentItem{Title: "Episode A", URL: "feed-entry"}, 1, 1)
	s := &Scheduler{downloader: &mockSchedDownloader{tasks: []core.DownloadTask{{Hash: "real-hash", Name: "Episode A", Status: "completed", Progress: 1}}}}
	s.pollDownloads(context.Background())
	var ep database.Episode
	database.DB.First(&ep)
	if ep.Status != "downloaded" || ep.TorrentHash != "real-hash" {
		t.Fatalf("占位编号未回填 %+v", ep)
	}
}

type fixtureCatalog struct{ subject string }

func (f fixtureCatalog) SearchAnime(context.Context, string) ([]core.TorrentItem, error) {
	return []core.TorrentItem{{Title: "Test anime", BangumiID: "33"}, {Title: "Test anime sequel", BangumiID: "55"}}, nil
}
func (f fixtureCatalog) GetAnimeInfo(context.Context, string) (*source.MikanAnimeInfo, error) {
	return &source.MikanAnimeInfo{SubjectID: f.subject}, nil
}
func TestCrossSiteMappingChecksSubject(t *testing.T) {
	sub := database.Subscription{TitleCN: "Test anime", MetadataProvider: "BGM.tv", MetadataID: "44"}
	if id := resolveMikanID(context.Background(), fixtureCatalog{"44"}, sub); id != "33" {
		t.Fatalf("应关联蜜柑编号 33，得到 %q", id)
	}
	if id := resolveMikanID(context.Background(), fixtureCatalog{"55"}, sub); id != "" {
		t.Fatalf("不应关联元数据不符的同名条目 %q", id)
	}
}
func TestCollectionWithNoMikanMappingIsRetained(t *testing.T) {
	subscriptionTestDB(t)
	s := &Scheduler{}
	a := core.Anime{TitleCN: "作品", ID: "200", Provider: "BGM.tv"}
	id, err := s.autoSubscribe(context.Background(), a)
	if err != nil || id == 0 {
		t.Fatal(id, err)
	}
	_, err = s.autoSubscribe(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	var subs []database.Subscription
	database.DB.Find(&subs)
	if len(subs) != 1 || subs[0].MetadataID != "200" || subs[0].BangumiID != "" {
		t.Fatalf("未映射收藏应保留并去重 %+v", subs)
	}
}
func TestMikanAvailableInsideProductionSourceWrapper(t *testing.T) {
	m := source.NewMikanSource("unused.invalid", "", nil)
	s := Scheduler{source: source.NewMultiSource(m)}
	if s.mikanSource() != m {
		t.Fatal("聚合源中未找到蜜柑详情客户端")
	}
}

type calendarFixture struct {
	core.MetadataProvider
	fetches int
	day     time.Time
}

func (*calendarFixture) Name() string { return "BGM.tv" }
func (f *calendarFixture) GetEpisodes(_ context.Context, id string, season int) ([]core.Episode, error) {
	f.fetches++
	return []core.Episode{{AnimeID: id, Season: season, Number: 1, AiredAt: f.day}}, nil
}
func TestSubscriptionPollCanAlignAbsoluteEpisode(t *testing.T) {
	subscriptionTestDB(t)
	day := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	md := &calendarFixture{day: day}
	src := &releaseFixture{items: []core.TorrentItem{{Title: "[Group] Calendar Show - 25", URL: "test", PublishedAt: day.Add(8 * time.Hour)}}}
	sub := database.Subscription{TitleCN: "Calendar Show", Season: 2, TotalEpisodes: 12, MetadataID: "50", MetadataProvider: "BGM.tv"}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	s := Scheduler{metadataProvider: md, source: src, mikanRSSURL: "fixture", downloader: &mockSchedDownloader{}}
	s.pollRSS(context.Background())
	var ep database.Episode
	if err := database.DB.First(&ep).Error; err != nil {
		t.Fatal(err)
	}
	if md.fetches != 1 || ep.Season != 2 || ep.Number != 1 {
		t.Fatalf("未按日历对齐 %+v", ep)
	}
}

type legacyCatalog struct{ hasMatch bool }

func (f legacyCatalog) SearchAnime(context.Context, string) ([]core.TorrentItem, error) {
	if !f.hasMatch {
		return nil, nil
	}
	return []core.TorrentItem{{BangumiID: "70", Title: "Existing Show"}}, nil
}
func (f legacyCatalog) GetAnimeInfo(_ context.Context, id string) (*source.MikanAnimeInfo, error) {
	if id == "70" {
		return &source.MikanAnimeInfo{SubjectID: "33", Title: "Existing Show"}, nil
	}
	return &source.MikanAnimeInfo{SubjectID: "99", Title: "Unrelated Show"}, nil
}
func TestLegacyCollectionIDsAreVerifiedBeforeHistory(t *testing.T) {
	for _, provider := range []string{"", "BGM.tv"} {
		sub := database.Subscription{BangumiID: "33", TitleCN: "Existing Show"}
		if provider != "" {
			sub.MetadataProvider, sub.MetadataID = provider, "33"
		}
		if id := verifiedMikanID(context.Background(), legacyCatalog{false}, sub); id != "" {
			t.Fatalf("未确认的旧编号被用于补全 %q", id)
		}
		if id := verifiedMikanID(context.Background(), legacyCatalog{true}, sub); id != "70" {
			t.Fatalf("应通过正确条目找回蜜柑 ID，得到 %q", id)
		}
	}
	subscriptionTestDB(t)
	old := database.Subscription{BangumiID: "33", TitleCN: "Existing Show", RSSURL: "old-feed"}
	if err := database.DB.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	_, err := (&Scheduler{}).autoSubscribe(context.Background(), core.Anime{Provider: "BGM.tv", ID: "33", TitleCN: old.TitleCN})
	if err != nil {
		t.Fatal(err)
	}
	var rows []database.Subscription
	database.DB.Find(&rows)
	if len(rows) != 1 || rows[0].ID != old.ID || rows[0].BangumiID != "" || rows[0].RSSURL != "" || rows[0].MetadataID != "33" {
		t.Fatal(rows)
	}
}
func TestBilingualReleaseAliases(t *testing.T) {
	sub := database.Subscription{TitleCN: "中文片名", TitleJP: "日本語名", TitleEN: "Calendar Show", Season: 1}
	for _, title := range []string{"[组] 中文片名 / 日本語名 - 01 [1080p]", "[组] 中文片名／日本語名 - 01", "[Group] Calendar Show S01E01", "[Group] [中文片名] [01] [1080p]"} {
		parsed := source.ParseMikanTitle(title)
		if !matchesRelease(parsed, sub) {
			t.Errorf("未识别双语片名 %q -> %q", title, parsed.Title)
		}
	}
	if matchesRelease(source.ParseMikanTitle("[Group] Fate/Zero - 03"), database.Subscription{TitleEN: "Fate"}) {
		t.Fatal("误匹配前缀片名")
	}
}
