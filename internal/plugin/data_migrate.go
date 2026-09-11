package plugin

import (
	"log"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// DataMigratePlugin 实现 BuiltInPlugin 接口，提供 AutoBangumi / ani-rss 历史数据迁移与导入能力
type DataMigratePlugin struct {
	mu                sync.RWMutex
	enabled           bool
	lastMigratedAt    *time.Time
	migratedSubs      int
	migratedEpisodes  int
	migratedDownloads int
	subID             core.SubscriptionID
}

func (p *DataMigratePlugin) GetInfo() PluginInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	desc := "支持从 AutoBangumi 或 ani-rss 的 SQLite 数据库文件中导入历史订阅、剧集状态和下载记录。"
	if p.lastMigratedAt != nil {
		desc += " [最近导入: " + p.lastMigratedAt.Format("2006-01-02 15:04:05") + "]"
	}

	return PluginInfo{
		ID:          "data-migrate",
		Name:        "数据导入与迁移",
		Description: desc,
		Version:     "1.1.0",
		Author:      "xiaoyue",
		AuthorURL:   "https://github.com/xiaoyueRX",
		Icon:        "Database",
		IsBuiltIn:   true,
		Type:        "builtin",
		Events:      []string{"data.migrate"},
	}
}

func (p *DataMigratePlugin) Init(bus core.EventBus, ctx core.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if bus != nil {
		if p.subID != 0 {
			bus.Unsubscribe("data.migrate", p.subID)
		}
		p.subID = bus.Subscribe("data.migrate", func(ev core.Event) {
			p.mu.Lock()
			defer p.mu.Unlock()
			if !p.enabled {
				return
			}
			now := time.Now()
			p.lastMigratedAt = &now
			if subs, ok := ev.Payload["subscriptions"].(int); ok {
				p.migratedSubs += subs
			}
			if eps, ok := ev.Payload["episodes"].(int); ok {
				p.migratedEpisodes += eps
			}
			if dls, ok := ev.Payload["downloads"].(int); ok {
				p.migratedDownloads += dls
			}
			log.Printf("🔌 [数据迁移插件] 记录迁移事件: +%d 订阅, +%d 剧集, +%d 下载",
				p.migratedSubs, p.migratedEpisodes, p.migratedDownloads)
		})
	}

	p.enabled = true
	log.Println("🔌 [插件] 数据导入与迁移插件已启用 (支持 AutoBangumi / ani-rss SQLite 迁移)")
	return nil
}

func (p *DataMigratePlugin) Stop(bus core.EventBus) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if bus != nil && p.subID != 0 {
		bus.Unsubscribe("data.migrate", p.subID)
		p.subID = 0
	}
	p.enabled = false
	log.Println("🔌 [插件] 数据导入与迁移插件已停用")
	return nil
}

func (p *DataMigratePlugin) GetStats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return map[string]interface{}{
		"last_migrated_at":   p.lastMigratedAt,
		"migrated_subs":      p.migratedSubs,
		"migrated_episodes":  p.migratedEpisodes,
		"migrated_downloads": p.migratedDownloads,
	}
}
