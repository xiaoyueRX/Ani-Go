package scheduler

import (
	"context"
	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

type subscriptionFeedFixture struct {
	core.Source
	items     []core.TorrentItem
	urls      []string
	historyID string
}

func (f *subscriptionFeedFixture) FetchRSS(_ context.Context, url string) ([]core.TorrentItem, error) {
	f.urls = append(f.urls, url)
	return f.items, nil
}
func (f *subscriptionFeedFixture) FetchHistory(_ context.Context, id string, _ core.Filter) ([]core.TorrentItem, error) {
	f.historyID = id
	return f.items, nil
}

type feedDownloader struct {
	mockSchedDownloader
	items []core.TorrentItem
}

func (d *feedDownloader) Add(_ context.Context, item core.TorrentItem, _ string) error {
	d.items = append(d.items, item)
	return nil
}

func TestSubscriptionFeedUsesItsURLAndAliases(t *testing.T) {
	for _, perSubscription := range []bool{true, false} {
		name := "全局英文名"
		if perSubscription {
			name = "独立订阅源"
		}
		t.Run(name, func(t *testing.T) {
			if err := database.Init(filepath.Join(t.TempDir(), "feeds.db")); err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{}
			cfg.Mikan.RSSMode = core.RSSModePersonal
			feed := &subscriptionFeedFixture{items: []core.TorrentItem{{Title: "[Group] Example Show - 01 [1080p]", URL: "https://example.test/1.torrent"}}}
			dl := &feedDownloader{}
			s := &Scheduler{cfg: cfg, source: feed, downloader: dl}
			url := "https://example.test/feed"
			sub := database.Subscription{TitleCN: "甲作品", TitleEN: "Example Show"}
			if perSubscription {
				sub.RSSURL = url
			} else {
				s.mikanRSSURL = url
			}
			if e := database.DB.Create(&sub).Error; e != nil {
				t.Fatal(e)
			}
			s.pollRSS(context.Background())
			if len(feed.urls) != 1 || feed.urls[0] != url || len(dl.items) != 1 {
				t.Fatalf("实际抓取 %v，下载 %d", feed.urls, len(dl.items))
			}
		})
	}
}

func TestSupplementRejectsUnrelatedReleases(t *testing.T) {
	if err := database.Init(filepath.Join(t.TempDir(), "history.db")); err != nil {
		t.Fatal(err)
	}
	sub := database.Subscription{TitleCN: "甲作品", BangumiID: "101", FilterJSON: `{"resolution":"1080p"}`}
	if e := database.DB.Create(&sub).Error; e != nil {
		t.Fatal(e)
	}
	feed := &subscriptionFeedFixture{items: []core.TorrentItem{
		{Title: "[Group] 完全不同作品 - 03 [1080p]", URL: "https://example.test/bad.torrent", SourceName: "Nyaa"},
		{Title: "[Group] 甲作品 - 03 [720p]", URL: "https://example.test/720.torrent", SourceName: "Nyaa"},
		{Title: "[Group] 甲作品 - 03 [1080p]", URL: "https://example.test/good.torrent", SourceName: "Nyaa"},
	}}
	dl := &feedDownloader{}
	s := &Scheduler{cfg: &config.Config{}, source: feed, downloader: dl}
	s.supplementOne(context.Background(), sub)
	if len(dl.items) != 1 || dl.items[0].URL != "https://example.test/good.torrent" {
		t.Fatalf("错误的下载列表 %+v", dl.items)
	}
}

func TestSubscriptionRegexUsedByRSS(t *testing.T) {
	if err := database.Init(filepath.Join(t.TempDir(), "regex.db")); err != nil {
		t.Fatal(err)
	}
	if err := source.SetCustomRegexPatterns([]string{`ReleaseNo([0-9]+)`}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.SetCustomRegexPatterns(nil) })
	sub := database.Subscription{TitleCN: "甲作品"}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Mikan.RSSMode = core.RSSModePersonal
	feed := &subscriptionFeedFixture{items: []core.TorrentItem{{Title: "[Group] 甲作品 ReleaseNo26", URL: "https://example.test/26.torrent"}}}
	s := &Scheduler{cfg: cfg, source: feed, downloader: &feedDownloader{}, mikanRSSURL: "https://example.test/feed"}
	s.pollRSS(context.Background())
	var ep database.Episode
	if err := database.DB.First(&ep).Error; err != nil {
		t.Fatal(err)
	}
	if ep.Number != 26 {
		t.Fatalf("实际集号 %v", ep.Number)
	}
}

type feedMetadata struct {
	core.MetadataProvider
	lookedUp   string
	calendarID string
	releases   []core.Episode
}

func (p *feedMetadata) Name() string { return "TMDB" }
func (p *feedMetadata) SearchAnime(_ context.Context, q string) ([]core.Anime, error) {
	return []core.Anime{{TitleCN: q, ID: "765432", Provider: "TMDB"}}, nil
}
func (p *feedMetadata) GetAnime(_ context.Context, id string) (core.Anime, error) {
	p.lookedUp = id
	return core.Anime{}, nil
}
func (p *feedMetadata) GetEpisodes(_ context.Context, id string, _ int) ([]core.Episode, error) {
	p.calendarID = id
	return p.releases, nil
}

func TestSupplementUsesProviderSpecificIDs(t *testing.T) {
	if err := database.Init(filepath.Join(t.TempDir(), "ids.db")); err != nil {
		t.Fatal(err)
	}
	sub := database.Subscription{TitleCN: "甲作品", BangumiID: "9988"}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	feed := &subscriptionFeedFixture{}
	md := &feedMetadata{}
	s := &Scheduler{cfg: &config.Config{}, source: feed, downloader: &feedDownloader{}, metadataProvider: md}
	s.supplementOne(context.Background(), sub)
	if err := database.DB.First(&sub, sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if md.lookedUp != "765432" || feed.historyID != "" || sub.MetadataID != "765432" || sub.MikanID != "" {
		t.Fatalf("元数据请求 %q；蜜柑请求 %q；订阅 %+v", md.lookedUp, feed.historyID, sub)
	}
	sub.MikanID = "55"
	s.supplementOne(context.Background(), sub)
	if md.lookedUp != "765432" || feed.historyID != "55" {
		t.Fatalf("元数据请求 %q，蜜柑请求 %q", md.lookedUp, feed.historyID)
	}
}

func TestAmbiguousSubscriptionEpisodeUsesVerifiedCalendar(t *testing.T) {
	if err := database.Init(filepath.Join(t.TempDir(), "calendar.db")); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2024, 7, 5, 8, 0, 0, 0, time.UTC)
	md := &feedMetadata{releases: []core.Episode{{Season: 2, Number: 1, AiredAt: now.Add(-8 * time.Hour)}}}
	s := &Scheduler{metadataProvider: md}
	sub := database.Subscription{TitleCN: "甲作品", Season: 2, TotalEpisodes: 12, MetadataProvider: "TMDB", MetadataID: "123"}
	season, num := s.resolveSubscriptionEpisode(context.Background(), core.TorrentItem{Title: "甲作品 - 25 [1080p]", PublishedAt: now}, sub)
	if md.calendarID != "123" || season != 2 || num != 1 {
		t.Fatalf("日历标识 %q，实际 S%dE%v", md.calendarID, season, num)
	}
}

type feedAI struct {
	ai.Classifier
	calls int
}

func (p *feedAI) DisambiguateTitle(context.Context, string, string, time.Time) (*ai.DisambiguateResult, error) {
	p.calls++
	return &ai.DisambiguateResult{Season: 2, Episode: 7, Confidence: 0.95}, nil
}

func TestAmbiguousSubscriptionEpisodeUsesEnabledAIAndCache(t *testing.T) {
	if err := database.Init(filepath.Join(t.TempDir(), "ai.db")); err != nil {
		t.Fatal(err)
	}
	client := &feedAI{}
	s := &Scheduler{aiClient: client}
	sub := database.Subscription{TitleCN: "甲作品", Season: 2}
	item := core.TorrentItem{Title: "[字幕组] 甲作品 最新话"}
	t.Setenv("AI_AUTO_DISAMBIGUATE", "false")
	if _, ep := s.resolveSubscriptionEpisode(context.Background(), item, sub); ep != 0 || client.calls != 0 {
		t.Fatalf("关闭 AI 时出现结果 %v", ep)
	}
	t.Setenv("AI_AUTO_DISAMBIGUATE", "true")
	for i := 0; i < 2; i++ {
		season, ep := s.resolveSubscriptionEpisode(context.Background(), item, sub)
		if ep != 7 || season != 2 {
			t.Fatalf("AI 结果 S%dE%v", season, ep)
		}
	}
	if client.calls != 1 {
		t.Fatalf("AI 被调用 %d 次", client.calls)
	}
}

func TestSharedFeedAndHistoryResolveIdenticalTitlesAcrossSeasons(t *testing.T) {
	for _, history := range []bool{false, true} {
		name := "共享 RSS"
		if history {
			name = "历史搜索"
		}
		t.Run(name, func(t *testing.T) {
			if err := database.Init(filepath.Join(t.TempDir(), "two-seasons.db")); err != nil {
				t.Fatal(err)
			}
			first := database.Subscription{TitleCN: "Harbor Story", Season: 1, MikanID: "101"}
			second := database.Subscription{TitleCN: "Harbor Story", Season: 2, MikanID: "102"}
			for _, sub := range []*database.Subscription{&first, &second} {
				if err := database.DB.Create(sub).Error; err != nil {
					t.Fatal(err)
				}
			}
			releases := []core.TorrentItem{
				{Title: "[Group] Harbor Story - 02 [1080p]", URL: "https://local.test/ambiguous.torrent", SourceName: "Nyaa"},
				{Title: "[Group] Harbor Story S02E03 [1080p]", URL: "https://local.test/labeled.torrent", SourceName: "Nyaa"},
				{Title: "[Group] Harbor Story - 04 [1080p]", URL: "https://local.test/known.torrent", SourceName: "Mikan", BangumiID: "102"},
				{Title: "[Group] Harbor Story S02E05 [1080p]", URL: "https://local.test/mismatched.torrent", SourceName: "Mikan", BangumiID: "101"},
			}
			feed := &subscriptionFeedFixture{items: releases}
			dl := &feedDownloader{}
			cfg := &config.Config{}
			cfg.Mikan.RSSMode = core.RSSModePersonal
			s := &Scheduler{cfg: cfg, source: feed, downloader: dl, mikanRSSURL: "https://local.test/global"}
			if history {
				if err := s.supplementOne(t.Context(), second); err != nil {
					t.Fatal(err)
				}
			} else {
				s.pollRSS(t.Context())
			}
			var episodes []database.Episode
			if err := database.DB.Find(&episodes).Error; err != nil {
				t.Fatal(err)
			}
			if len(episodes) != 2 {
				t.Fatalf("错误的任务数 %d, 下载为 %+v", len(episodes), dl.items)
			}
			for _, ep := range episodes {
				if ep.SubscriptionID != second.ID || ep.Season != 2 || ep.TorrentURL == releases[0].URL {
					t.Errorf("错误归属 %+v", ep)
				}
			}
		})
	}
}

func TestPrivateSeasonFeedAcceptsReleaseWithoutSeasonMarker(t *testing.T) {
	if err := database.Init(filepath.Join(t.TempDir(), "scoped.db")); err != nil {
		t.Fatal(err)
	}
	first := database.Subscription{TitleCN: "Harbor Story", Season: 1}
	second := database.Subscription{TitleCN: "Harbor Story", Season: 2, RSSURL: "https://local.test/season-two"}
	for _, sub := range []*database.Subscription{&first, &second} {
		if err := database.DB.Create(sub).Error; err != nil {
			t.Fatal(err)
		}
	}
	feed := &subscriptionFeedFixture{items: []core.TorrentItem{{Title: "[Group] Harbor Story - 03", URL: "https://local.test/third.torrent"}}}
	dl := &feedDownloader{}
	s := &Scheduler{cfg: &config.Config{}, source: feed, downloader: dl}
	s.pollRSS(t.Context())
	var ep database.Episode
	if err := database.DB.First(&ep).Error; err != nil || ep.SubscriptionID != second.ID || ep.Season != 2 {
		t.Fatalf("独立订阅源未正确归季：%+v, %v", ep, err)
	}
}
