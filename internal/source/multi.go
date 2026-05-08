package source

import (
	"context"
	"log"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// MultiSource 聚合多个资源站，按优先级依次查询

type MultiSource struct {
	sources []core.Source
}

func NewMultiSource(sources ...core.Source) *MultiSource {
	filtered := make([]core.Source, 0, len(sources))
	for _, s := range sources {
		if s != nil {
			filtered = append(filtered, s)
		}
	}
	ms := &MultiSource{sources: filtered}
	if len(filtered) > 0 {
		names := make([]string, len(filtered))
		for i, s := range filtered {
			names[i] = s.Name()
		}
		log.Printf("📡 MultiSource 已聚合 %d 个资源站: %v", len(filtered), names)
	}
	return ms
}

func (ms *MultiSource) Name() string { return "MultiSource" }

func (ms *MultiSource) IsAvailable(ctx context.Context) bool {
	for _, s := range ms.sources {
		if s.IsAvailable(ctx) {
			return true
		}
	}
	return false
}

// AddSource 动态添加资源站
func (ms *MultiSource) AddSource(s core.Source) {
	ms.sources = append(ms.sources, s)
}

// Sources 返回所有已注册的资源站
func (ms *MultiSource) Sources() []core.Source {
	return ms.sources
}

func (ms *MultiSource) FetchRSS(ctx context.Context, url string) ([]core.TorrentItem, error) {
	// 依次尝试所有资源站，第一个成功的返回
	var lastErr error
	for _, s := range ms.sources {
		if !s.IsAvailable(ctx) {
			continue
		}
		items, err := s.FetchRSS(ctx, url)
		if err != nil {
			lastErr = err
			continue
		}
		if len(items) > 0 {
			return items, nil
		}
	}
	return nil, lastErr
}

func (ms *MultiSource) SearchAnime(ctx context.Context, title string) ([]core.TorrentItem, error) {
	return ms.searchAll(ctx, func(s core.Source) ([]core.TorrentItem, error) {
		return s.SearchAnime(ctx, title)
	}, title)
}

func (ms *MultiSource) FetchHistory(ctx context.Context, bangumiID string, filter core.Filter) ([]core.TorrentItem, error) {
	return ms.searchAll(ctx, func(s core.Source) ([]core.TorrentItem, error) {
		return s.FetchHistory(ctx, bangumiID, filter)
	}, filter.PreferSubgroup)
}

// searchAll 在所有可用资源站上执行搜索，合并结果
func (ms *MultiSource) searchAll(ctx context.Context, search func(core.Source) ([]core.TorrentItem, error), query string) ([]core.TorrentItem, error) {
	if len(ms.sources) == 0 {
		return nil, nil
	}

	allItems := make([]core.TorrentItem, 0)
	seen := make(map[string]bool)

	var lastErr error
	for _, s := range ms.sources {
		if !s.IsAvailable(ctx) {
			continue
		}

		items, err := search(s)
		if err != nil {
			log.Printf("⚠️  资源站 [%s] 搜索失败: %v", s.Name(), err)
			lastErr = err
			continue
		}

		for _, item := range items {
			dedupeKey := item.URL
			if dedupeKey == "" {
				dedupeKey = item.Title
			}
			if seen[dedupeKey] {
				continue
			}
			seen[dedupeKey] = true
			allItems = append(allItems, item)
		}
	}

	if len(allItems) == 0 && lastErr != nil {
		return nil, lastErr
	}

	return allItems, nil
}
