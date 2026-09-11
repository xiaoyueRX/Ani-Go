package plugin

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/metadata"
)

type MetadataMappingPlugin struct {
	mu          sync.RWMutex
	enabled     bool
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
		p.mu.RLock()
		active := p.enabled
		p.mu.RUnlock()
		if !active {
			return
		}
		subID := parseSubID(ev)
		if subID > 0 { go p.mapOne(subID) }
	})

	p.subSuppID = bus.Subscribe(core.EventSupplementTriggered, func(ev core.Event) {
		p.mu.RLock()
		active := p.enabled
		p.mu.RUnlock()
		if !active {
			return
		}
		subID := parseSubID(ev)
		if subID > 0 { go p.mapOne(subID) }
	})
	p.initialized = true
	p.enabled = true
	log.Println("🔌 [插件] 元数据映射器已启用")
	return nil
}

func (p *MetadataMappingPlugin) Stop(bus core.EventBus) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		bus.Unsubscribe(core.EventSubscriptionAdded, p.subAddedID)
		bus.Unsubscribe(core.EventSupplementTriggered, p.subSuppID)
		p.initialized = false
	}
	p.enabled = false
	log.Println("🔌 [插件] 元数据映射器已停用，已注销事件监听")
	return nil
}

func parseSubID(ev core.Event) uint {
	if id, ok := ev.Payload["subscription_id"].(uint); ok { return id }
	if f, ok := ev.Payload["subscription_id"].(float64); ok { return uint(f) }
	return 0
}

func (p *MetadataMappingPlugin) mapOne(subID uint) {
	p.mu.RLock()
	active := p.enabled
	p.mu.RUnlock()
	if !active {
		return
	}

	var sub database.Subscription
	if err := database.DB.First(&sub, subID).Error; err != nil || sub.BangumiID == "" {
		return
	}
	if sub.TMDBID != "" && sub.IMDBID != "" { return }

	log.Printf("🔗 [插件] 正在寻找 [%s] 的外部 ID 映射...", sub.TitleCN)
	
	// 内部创建映射器：若配置了 TMDB_API_KEY 则注入 TMDBProvider 实现自动搜索兜底
	var tmdbProvider *metadata.TMDBProvider
	tmdbKey := ""
	if database.DB != nil {
		var s database.Setting
		if err := database.DB.Where("key = ?", "TMDB_API_KEY").First(&s).Error; err == nil {
			tmdbKey = strings.TrimSpace(s.Value)
		}
	}
	if tmdbKey == "" {
		tmdbKey = strings.TrimSpace(os.Getenv("TMDB_API_KEY"))
	}
	if tmdbKey != "" {
		lang := "zh-CN"
		var mirrors []string
		if database.DB != nil {
			var s database.Setting
			if err := database.DB.Where("key = ?", "TMDB_MIRROR_DOMAINS").First(&s).Error; err == nil && s.Value != "" {
				for _, m := range strings.Split(s.Value, ",") {
					if tr := strings.TrimSpace(m); tr != "" {
						mirrors = append(mirrors, tr)
					}
				}
			}
		}
		tmdbProvider = metadata.NewTMDBProvider(tmdbKey, lang, mirrors)
	}

	mapper := metadata.NewMapper(tmdbProvider)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tmdb, imdb := mapper.MapBangumiToExternal(ctx, sub.BangumiID, sub.TitleCN, sub.Year)
	
	if tmdb != "" || imdb != "" {
		database.DB.Model(&sub).Updates(map[string]interface{}{
			"tmdb_id": tmdb,
			"imdb_id": imdb,
		})
		log.Printf("✅ [插件] 关联成功 [%s]: TMDB=%s, IMDB=%s", sub.TitleCN, tmdb, imdb)
	}
}
