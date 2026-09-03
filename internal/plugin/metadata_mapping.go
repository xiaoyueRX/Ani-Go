package plugin

import (
	"context"
	"log"
	"sync"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/metadata"
)

type MetadataMappingPlugin struct {
	mu          sync.Mutex
	initialized bool
	subAddedID  core.SubscriptionID
	subSuppID   core.SubscriptionID
}

func (p *MetadataMappingPlugin) GetInfo() PluginInfo {
	return PluginInfo{
		ID:          "metadata-mapping",
		Name:        "元数据映射器",
		Description: "自动关联 BangumiID 到 TMDB/IMDB，打通全平台追番信息。",
		Version:     "1.1.0",
		Author:      "xiaoyue",
		AuthorURL:   "https://github.com/xiaoyueRX",
		Icon:        "Link",
		IsBuiltIn:   true,
		Type:        "builtin",
		Events:      []string{core.EventSubscriptionAdded, core.EventSupplementTriggered},
	}
}

func (p *MetadataMappingPlugin) Init(bus core.EventBus, ctx core.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		bus.Unsubscribe(core.EventSubscriptionAdded, p.subAddedID)
		bus.Unsubscribe(core.EventSupplementTriggered, p.subSuppID)
	}

	// 监听订阅添加和补全触发事件
	p.subAddedID = bus.Subscribe(core.EventSubscriptionAdded, func(ev core.Event) {
		subID := parseSubID(ev)
		if subID > 0 { go p.mapOne(subID) }
	})

	p.subSuppID = bus.Subscribe(core.EventSupplementTriggered, func(ev core.Event) {
		subID := parseSubID(ev)
		if subID > 0 { go p.mapOne(subID) }
	})
	p.initialized = true
	return nil
}

func parseSubID(ev core.Event) uint {
	if id, ok := ev.Payload["subscription_id"].(uint); ok { return id }
	if f, ok := ev.Payload["subscription_id"].(float64); ok { return uint(f) }
	return 0
}

func (p *MetadataMappingPlugin) mapOne(subID uint) {
	var sub database.Subscription
	if err := database.DB.First(&sub, subID).Error; err != nil || sub.BangumiID == "" {
		return
	}
	if sub.TMDBID != "" && sub.IMDBID != "" { return }

	log.Printf("🔗 [插件] 正在寻找 [%s] 的外部 ID 映射...", sub.TitleCN)
	
	// 内部创建映射器（插件自包含逻辑）
	mapper := metadata.NewMapper(nil)
	tmdb, imdb := mapper.MapBangumiToExternal(context.Background(), sub.BangumiID, sub.TitleCN, sub.Year)
	
	if tmdb != "" || imdb != "" {
		database.DB.Model(&sub).Updates(map[string]interface{}{
			"tmdb_id": tmdb,
			"imdb_id": imdb,
		})
		log.Printf("✅ [插件] 关联成功 [%s]: TMDB=%s, IMDB=%s", sub.TitleCN, tmdb, imdb)
	}
}
