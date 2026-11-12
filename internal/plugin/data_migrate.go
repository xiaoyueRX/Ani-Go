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
	subBus            core.EventBus
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

// Init 启用数据迁移插件（幂等支持重复调用与 nil 总线保护）
func (p *DataMigratePlugin) Init(bus core.EventBus, ctx core.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.subID != 0 {
		unsubBus := p.subBus
		if unsubBus == nil {
			unsubBus = bus
		}
		if unsubBus != nil {
			unsubBus.Unsubscribe("data.migrate", p.subID)
		}
		p.subID = 0
		p.subBus = nil
	}

	if bus != nil {
		p.subBus = bus
		p.subID = bus.Subscribe("data.migrate", func(ev core.Event) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("⚠️ [数据迁移插件] 处理事件异常崩溃捕获: %v", r)
				}
			}()
			if ev.Payload == nil {
				return
			}
			p.mu.Lock()
			defer p.mu.Unlock()
			if !p.enabled {
				return
			}
			now := time.Now()
			p.lastMigratedAt = &now
			subs := extractPayloadInt(ev.Payload["subscriptions"])
			eps := extractPayloadInt(ev.Payload["episodes"])
			dls := extractPayloadInt(ev.Payload["downloads"])
			p.migratedSubs += subs
			p.migratedEpisodes += eps
			p.migratedDownloads += dls
			log.Printf("🔌 [数据迁移插件] 记录迁移事件: +%d 订阅, +%d 剧集, +%d 下载 (累计: %d/%d/%d)",
				subs, eps, dls, p.migratedSubs, p.migratedEpisodes, p.migratedDownloads)
		})
	}

	p.enabled = true
	log.Println("🔌 [插件] 数据导入与迁移插件已启用 (支持 AutoBangumi / ani-rss SQLite 迁移)")
	return nil
}

// Stop 停用数据迁移插件并注销事件监听（幂等安全）
func (p *DataMigratePlugin) Stop(bus core.EventBus) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	wasEnabled := p.enabled
	unsubBus := bus
	if unsubBus == nil {
		unsubBus = p.subBus
	}
	if unsubBus != nil && p.subID != 0 {
		unsubBus.Unsubscribe("data.migrate", p.subID)
	}
	p.subID = 0
	p.subBus = nil
	p.enabled = false
	if wasEnabled {
		log.Println("🔌 [插件] 数据导入与迁移插件已停用")
	}
	return nil
}

// GetStats 返回迁移统计快照
func (p *DataMigratePlugin) GetStats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var lastCopy *time.Time
	if p.lastMigratedAt != nil {
		t := *p.lastMigratedAt
		lastCopy = &t
	}
	return map[string]interface{}{
		"last_migrated_at":   lastCopy,
		"migrated_subs":      p.migratedSubs,
		"migrated_episodes":  p.migratedEpisodes,
		"migrated_downloads": p.migratedDownloads,
	}
}

func extractPayloadInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
