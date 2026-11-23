package scheduler

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
	"gorm.io/gorm"
)

func deliveryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.Episode{}, &database.Subscription{}, &database.Setting{}); err != nil {
		t.Fatal(err)
	}
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	return db
}

type deliveryHistory struct {
	core.Source
	err error
}

func (s deliveryHistory) FetchHistory(context.Context, string, core.Filter) ([]core.TorrentItem, error) {
	return nil, s.err
}

type capturedEvents struct {
	core.EventBus
	events []core.Event
}

func (b *capturedEvents) Publish(ev core.Event) { b.events = append(b.events, ev) }

func TestSupplementFinishesEmptyAndFailedScans(t *testing.T) {
	db := deliveryTestDB(t)
	sub := database.Subscription{TitleCN: "测试", MikanID: "1"}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	for _, historyErr := range []error{nil, errors.New("来源不可用")} {
		bus := &capturedEvents{}
		s := &Scheduler{bus: bus, source: deliveryHistory{err: historyErr}}
		s.supplementOne(context.Background(), sub)
		if len(bus.events) < 2 || bus.events[0].Type != core.EventSupplementTriggered {
			t.Fatalf("缺少起始事件: %v", bus.events)
		}
		last := bus.events[len(bus.events)-1]
		if last.Type != core.EventSupplementCompleted || last.Payload["new_count"] != 0 {
			t.Fatalf("错误的扫描终态: %+v", last)
		}
		if historyErr == nil && last.Payload["error"] != nil || historyErr != nil && last.Payload["error"] != historyErr.Error() {
			t.Fatalf("错误状态不匹配: %+v", last)
		}
	}
}

type deliverableDownload struct {
	core.Downloader
	path string
}

func (d deliverableDownload) GetStatus(context.Context, string) (core.DownloadTask, error) {
	return core.DownloadTask{ContentPath: d.path, Progress: 1, Status: "completed"}, nil
}

func TestAutomaticOrganizationCarriesNFOMetadata(t *testing.T) {
	db := deliveryTestDB(t)
	bus := event.New()
	p := plugin.NewNFOScraperPlugin()
	if err := p.Init(bus, nil); err != nil {
		t.Fatal(err)
	}
	defer p.Stop(bus)
	for _, tc := range []struct{ provider, id, bgm string }{{"BGM.tv", "100", "100"}, {"TMDB", "tv/200", ""}} {
		root := t.TempDir()
		show := filepath.Join(root, "show")
		if err := os.MkdirAll(show, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(show, "poster.jpg"), []byte("已存在的海报"), 0644); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "source.mkv")
		if err := os.WriteFile(path, []byte("内容"), 0644); err != nil {
			t.Fatal(err)
		}
		sub := database.Subscription{TitleCN: "测试", MetadataProvider: tc.provider, MetadataID: tc.id, MikanID: "7", BangumiID: "7", CoverURL: "https://example.invalid/poster.jpg", Description: "简介", TMDBID: "tv/200", IMDBID: "tt300", AnimeType: "tv"}
		if err := db.Create(&sub).Error; err != nil {
			t.Fatal(err)
		}
		ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, Title: "第一集", TorrentHash: tc.id, Status: "downloaded"}
		if err := db.Create(&ep).Error; err != nil {
			t.Fatal(err)
		}
		org := organizer.New("show/Season 1/E01", "", "", root, root, true, bus)
		s := &Scheduler{bus: bus, organizer: org, downloader: deliverableDownload{path: path}}
		s.pollOrganizer(context.Background())
		nfoPath := filepath.Join(show, "tvshow.nfo")
		deadline := time.Now().Add(2 * time.Second)
		var data []byte
		for {
			var err error
			data, err = os.ReadFile(nfoPath)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal(err)
			}
			time.Sleep(5 * time.Millisecond)
		}
		var result plugin.TVShowNFO
		if err := xml.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		if result.Plot != sub.Description || result.Thumb != sub.CoverURL || result.TMDBID != "200" || result.IMDBID != sub.IMDBID || result.BangumiID != tc.bgm {
			t.Fatalf("元数据不匹配: %+v", result)
		}
	}
}

type refusingDownload struct{ core.Downloader }

func (refusingDownload) Add(context.Context, core.TorrentItem, string) error {
	return errors.New("下载器拒绝添加")
}

func TestRejectedDownloadPublishesFailure(t *testing.T) {
	db := deliveryTestDB(t)
	sub := database.Subscription{TitleCN: "测试", CustomPath: t.TempDir()}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	bus := &capturedEvents{}
	s := &Scheduler{downloader: refusingDownload{}, bus: bus}
	item := core.TorrentItem{Title: "剧集 01", URL: "https://example.invalid/1.torrent"}
	if _, err := s.enqueue(context.Background(), sub, item, 1, 1); err == nil {
		t.Fatal("下载器拒绝却返回成功")
	}
	if len(bus.events) != 1 {
		t.Fatalf("预期恰好一个失败事件: %+v", bus.events)
	}
	ev := bus.events[0]
	if ev.Type != core.EventDownloadFailed || ev.Payload["subscription_id"] != sub.ID ||
		ev.Payload["title"] != item.Title || ev.Payload["error"] != "下载器拒绝添加" {
		t.Fatalf("失败事件内容错误: %+v", ev)
	}
}
