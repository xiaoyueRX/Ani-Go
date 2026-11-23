package source

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// MultiSource 聚合多个资源站，按优先级依次查询

type MultiSource struct {
	sources         []core.Source
	initSearchSlots sync.Once
	searchSlots     chan struct{}
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

func (ms *MultiSource) getActiveSources() []core.Source {
	if database.DB == nil {
		return ms.sources
	}
	var defaults config.SourcesConfig
	active := make([]core.Source, 0, len(ms.sources))
	for _, src := range ms.sources {
		switch src := src.(type) {
		case *NyaaSource:
			defaults.Nyaa = config.SourceConfig{Enabled: true, Domain: src.domain}
		case *ACGRIPSource:
			defaults.ACGRIP = config.SourceConfig{Enabled: true, Domain: src.domain}
		case *AnimeToshoSource:
			defaults.AnimeTosho = config.SourceConfig{Enabled: true, Domain: src.domain}
		default:
			active = append(active, src)
		}
	}

	var rows []database.Setting
	keys := []string{"NYAA_ENABLED", "NYAA_DOMAIN", "ACGRIP_ENABLED", "ACGRIP_DOMAIN", "ANIMETOSHO_ENABLED", "ANIMETOSHO_DOMAIN"}
	database.DB.Where("key IN ?", keys).Find(&rows)
	settings := make(map[string]string, len(rows))
	for _, row := range rows {
		settings[row.Key] = row.Value
	}
	defaults.ApplySettings(func(key string) (string, bool) {
		value, ok := settings[key]
		return value, ok
	})
	if defaults.Nyaa.Enabled {
		active = append(active, NewNyaaSource(defaults.Nyaa.Domain))
	}
	if defaults.ACGRIP.Enabled {
		active = append(active, NewACGRIPSource(defaults.ACGRIP.Domain))
	}
	if defaults.AnimeTosho.Enabled {
		active = append(active, NewAnimeToshoSource(defaults.AnimeTosho.Domain))
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
	// 一个订阅地址只向其主机发起一次请求，独立 RSS 不依赖站点首页可用性。
	for _, src := range ms.sources {
		if src.Name() == "Mikan" {
			return src.FetchRSS(ctx, url)
		}
	}
	if len(ms.sources) > 0 {
		return ms.sources[0].FetchRSS(ctx, url)
	}
	return nil, nil
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sources := ms.getActiveSources()
	if len(sources) == 0 {
		return nil, nil
	}

	type sourceResult struct {
		name  string
		items []core.TorrentItem
		err   error
	}

	// 同一服务的搜索与订阅补全共享额度，一个查询仍能并行访问所有内置来源。
	ms.initSearchSlots.Do(func() { ms.searchSlots = make(chan struct{}, 8) })
	resChan := make(chan sourceResult, len(sources))
	var wg sync.WaitGroup

startSources:
	for _, s := range sources {
		select {
		case ms.searchSlots <- struct{}{}:
		case <-ctx.Done():
			break startSources
		}
		wg.Add(1)
		go func(src core.Source) {
			defer wg.Done()
			defer func() { <-ms.searchSlots }()
			if ctx.Err() != nil {
				return
			}
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

	if len(allItems) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if lastErr != nil {
			return nil, lastErr
		}
	}
	return allItems, nil
}

// 无站内作品 ID 的源按订阅别名查找，禁止将字幕组或其他站点的 ID 作为作品名。
func searchHistoryTitles(ctx context.Context, titles []string, search func(context.Context, string) ([]core.TorrentItem, error)) ([]core.TorrentItem, error) {
	var lastErr error
	succeeded := false
	seen := make(map[string]bool)
	for _, title := range titles {
		title = strings.TrimSpace(title)
		key := core.NormalizeTitle(title)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		items, err := search(ctx, title)
		if err != nil {
			lastErr = err
			continue
		}
		succeeded = true
		if len(items) > 0 {
			return items, nil
		}
	}
	if succeeded {
		return nil, nil
	}
	return nil, lastErr
}

// MikanFrom 从聚合源或单一源取得已配置的蜜柑客户端。
func MikanFrom(src core.Source) *MikanSource {
	if m, ok := src.(*MikanSource); ok {
		return m
	}
	if multi, ok := src.(*MultiSource); ok {
		for _, s := range multi.sources {
			if m, ok := s.(*MikanSource); ok {
				return m
			}
		}
	}
	return nil
}
