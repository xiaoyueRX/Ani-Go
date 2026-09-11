package source

import (
	"context"
	"log"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
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

func (ms *MultiSource) isSourceEnabled(key string, defaultVal bool) bool {
	if database.DB == nil {
		return defaultVal
	}
	var s database.Setting
	if err := database.DB.Where("key = ?", key).First(&s).Error; err == nil {
		return s.Value == "true" || s.Value == "1"
	}
	return defaultVal
}

func (ms *MultiSource) getSetting(key string, defaultVal string) string {
	if database.DB == nil {
		return defaultVal
	}
	var s database.Setting
	if err := database.DB.Where("key = ?", key).First(&s).Error; err == nil && s.Value != "" {
		return s.Value
	}
	return defaultVal
}

func (ms *MultiSource) getActiveSources() []core.Source {
	if database.DB == nil {
		return ms.sources
	}

	var baseSources []core.Source
	hasNyaa := false
	hasACGRIP := false
	hasAnimeTosho := false

	for _, s := range ms.sources {
		name := s.Name()
		if name == "Nyaa" {
			hasNyaa = true
		} else if name == "ACG.RIP" {
			hasACGRIP = true
		} else if name == "AnimeTosho" {
			hasAnimeTosho = true
		} else {
			baseSources = append(baseSources, s)
		}
	}

	active := append([]core.Source{}, baseSources...)

	if ms.isSourceEnabled("NYAA_ENABLED", hasNyaa) {
		domain := ms.getSetting("NYAA_DOMAIN", "nyaa.si")
		active = append(active, NewNyaaSource(domain))
	}

	if ms.isSourceEnabled("ACGRIP_ENABLED", hasACGRIP) {
		domain := ms.getSetting("ACGRIP_DOMAIN", "acg.rip")
		active = append(active, NewACGRIPSource(domain))
	}

	if ms.isSourceEnabled("ANIMETOSHO_ENABLED", hasAnimeTosho) {
		domain := ms.getSetting("ANIMETOSHO_DOMAIN", "animetosho.org")
		active = append(active, NewAnimeToshoSource(domain))
	}

	if len(active) == 0 {
		return ms.sources
	}
	return active
}

func (ms *MultiSource) Name() string { return "MultiSource" }

func (ms *MultiSource) IsAvailable(ctx context.Context) bool {
	for _, s := range ms.getActiveSources() {
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

// Sources 返回所有当前启用的资源站
func (ms *MultiSource) Sources() []core.Source {
	return ms.getActiveSources()
}

func (ms *MultiSource) FetchRSS(ctx context.Context, url string) ([]core.TorrentItem, error) {
	// 依次尝试所有资源站，第一个成功的返回
	var lastErr error
	for _, s := range ms.getActiveSources() {
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

// searchAll 在所有可用资源站上并发执行搜索，合并结果
func (ms *MultiSource) searchAll(ctx context.Context, search func(core.Source) ([]core.TorrentItem, error), query string) ([]core.TorrentItem, error) {
	sources := ms.getActiveSources()
	if len(sources) == 0 {
		return nil, nil
	}

	type sourceResult struct {
		name  string
		items []core.TorrentItem
		err   error
	}

	resChan := make(chan sourceResult, len(sources))
	var wg sync.WaitGroup

	for _, s := range sources {
		wg.Add(1)
		go func(src core.Source) {
			defer wg.Done()
			items, err := search(src)
			resChan <- sourceResult{name: src.Name(), items: items, err: err}
		}(s)
	}

	wg.Wait()
	close(resChan)

	allItems := make([]core.TorrentItem, 0)
	seen := make(map[string]bool)
	var lastErr error

	for res := range resChan {
		if res.err != nil {
			log.Printf("⚠️  资源站 [%s] 搜索失败: %v", res.name, res.err)
			lastErr = res.err
			continue
		}

		for _, item := range res.items {
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
