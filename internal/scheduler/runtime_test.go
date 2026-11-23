package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

type runtimeRSS struct {
	pipelineSource
	fetched     chan struct{}
	returnAfter chan struct{}
}

func (s runtimeRSS) FetchRSS(ctx context.Context, _ string) ([]core.TorrentItem, error) {
	s.fetched <- struct{}{}
	if s.returnAfter != nil {
		<-ctx.Done()
		<-s.returnAfter
	}
	return nil, ctx.Err()
}

func TestSchedulerWaitsForInFlightRSSAndCanRestart(t *testing.T) {
	pipelineDB(t)
	cfg := config.Load()
	s := &Scheduler{cfg: cfg, source: runtimeRSS{fetched: make(chan struct{}, 4), returnAfter: make(chan struct{})}, mikanRSSURL: "fixture"}
	src := s.source.(runtimeRSS)
	ctx, cancel := context.WithCancel(context.Background())
	end := s.StartAsync(ctx)
	<-src.fetched
	if s.StartAsync(ctx) != end {
		t.Fatal("重复调用启动了第二组定时任务")
	}
	cancel()
	select {
	case <-end:
		t.Fatal("RSS 尚未退出却已宣告停止")
	case <-time.After(25 * time.Millisecond):
	}
	close(src.returnAfter)
	select {
	case <-end:
	case <-time.After(time.Second):
		t.Fatal("关闭未完成")
	}
	if err := s.TriggerSupplement(context.Background(), 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("停止后仍接收补全: %v", err)
	}
	next, stop := context.WithCancel(context.Background())
	done := s.StartAsync(next)
	<-src.fetched
	stop()
	<-done
}

func TestSavedIntervalRearmsSleepingScheduler(t *testing.T) {
	db := pipelineDB(t)
	cfg := config.Load()
	cfg.Scheduler.RSSInterval = time.Hour * 24
	s := &Scheduler{cfg: cfg, source: runtimeRSS{fetched: make(chan struct{}, 8)}, mikanRSSURL: "fixture"}
	ctx, cancel := context.WithCancel(context.Background())
	done := s.StartAsync(ctx)
	defer func() { cancel(); <-done }()
	ch := s.source.(runtimeRSS).fetched
	<-ch
	if err := db.Save(&database.Setting{Key: "RSS_INTERVAL", Value: "40ms"}).Error; err != nil {
		t.Fatal(err)
	}
	s.ReloadIntervals()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("缩短间隔后仍等待旧的 24 小时定时器")
	}
}

type progressHistory struct {
	pipelineSource
	requests int
}

func (s *progressHistory) FetchHistory(context.Context, string, core.Filter) ([]core.TorrentItem, error) {
	s.requests++
	return nil, nil
}
func TestSupplementRepairsStaleCompleteCountAcrossReleases(t *testing.T) {
	db := pipelineDB(t)
	src := &progressHistory{}
	s := Scheduler{source: src}
	sub := database.Subscription{TitleCN: "Series", Season: 1, TotalEpisodes: 2, CurrentEpisodes: 4, Completed: true, MikanID: "55"}
	savePipelineRow(t, db, &sub)
	for i, ep := range []database.Episode{
		{Season: 1, Number: 1, Status: "organized"}, {Season: 1, Number: 1, Status: "completed"},
		{Season: 0, Number: 1, Title: "SP01", Status: "organized"}, {Season: 1, Number: 1.5, Status: "organized"}, {Season: 2, Number: 2, Status: "organized"},
	} {
		ep.SubscriptionID = sub.ID
		ep.GroupName = string(rune('A' + i))
		savePipelineRow(t, db, &ep)
	}
	s.pollSupplement(context.Background())
	if err := db.First(&sub, sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if src.requests != 1 || sub.CurrentEpisodes != 1 || sub.Completed {
		t.Fatalf("同集多份和特典掩盖了缺失第二集: requests=%d sub=%+v", src.requests, sub)
	}
}

type blockedHistory struct {
	pipelineSource
	started chan struct{}
	resume  chan struct{}
}

func (s blockedHistory) FetchHistory(ctx context.Context, _ string, _ core.Filter) ([]core.TorrentItem, error) {
	s.started <- struct{}{}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.resume:
		return nil, errors.New("history unavailable")
	}
}
func TestOverlappingSupplementRequestsShareRunAndError(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", MikanID: "22"}
	savePipelineRow(t, db, &sub)
	src := blockedHistory{started: make(chan struct{}, 4), resume: make(chan struct{})}
	s := Scheduler{source: src}
	results := make(chan error, 2)
	go func() { results <- s.TriggerSupplement(context.Background(), sub.ID) }()
	<-src.started
	go func() { results <- s.TriggerSupplement(context.Background(), sub.ID) }()
	select {
	case <-src.started:
		t.Error("相同订阅同时发出两次历史查询")
	case <-time.After(40 * time.Millisecond):
	}
	close(src.resume)
	for range 2 {
		if err := <-results; err == nil || err.Error() != "history unavailable" {
			t.Errorf("丢失查询错误: %v", err)
		}
	}
}

func TestClearingPersistedSettingsDropsPreviousBootValues(t *testing.T) {
	db := pipelineDB(t)
	t.Setenv("RSS_INTERVAL", "")
	t.Setenv("MIKAN_RSS_URL", "")
	t.Setenv("SEED_CLEANUP_ENABLED", "")
	base := config.Load()
	boot := *base
	boot.MergeFromSettings(func(key string) (string, bool) {
		v, ok := map[string]string{"RSS_INTERVAL": "9h", "MIKAN_RSS_URL": "https://example.org/old-private", "SEED_CLEANUP_ENABLED": "false"}[key]
		return v, ok
	})
	s := New(&boot, pipelineSource{}, nil, nil, nil, nil, nil, base)
	for _, key := range []string{"RSS_INTERVAL", "MIKAN_RSS_URL", "SEED_CLEANUP_ENABLED"} {
		savePipelineRow(t, db, &database.Setting{Key: key, Value: ""})
	}
	if s.getMikanRSSURL() != base.Mikan.PersonalRSSURL || s.isSeedCleanupEnabled() != base.Scheduler.SeedCleanupEnabled {
		t.Fatal("清空后继续使用旧私有订阅地址/清理开关")
	}
	ticker := s.scheduleTicker("RSS_INTERVAL", s.runtimeConfig().Scheduler.RSSInterval)
	defer ticker.Stop()
	if ticker.interval != base.Scheduler.RSSInterval {
		t.Fatal("清除间隔后未恢复部署默认值")
	}
	t.Setenv("RSS_INTERVAL", "2h")
	if got := s.getInterval("RSS_INTERVAL", base.Scheduler.RSSInterval); got != 2*time.Hour {
		t.Fatalf("未优先采用部署设置，实际 %v", got)
	}
}
