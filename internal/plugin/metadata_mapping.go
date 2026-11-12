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

// MetadataMappingPlugin 元数据映射器插件，自动关联 BangumiID 与 TMDB / IMDB
type MetadataMappingPlugin struct {
	mu          sync.RWMutex
	enabled     bool
	initialized bool
	subBus      core.EventBus
	subAddedID  core.SubscriptionID
	subSuppID   core.SubscriptionID
	runCtx      context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
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

// Init 启用元数据映射插件（幂等支持重复调用，安全防 nil 总线）
func (p *MetadataMappingPlugin) Init(bus core.EventBus, ctx core.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		unsubBus := p.subBus
		if unsubBus == nil {
			unsubBus = bus
		}
		if unsubBus != nil {
			if p.subAddedID != 0 {
				unsubBus.Unsubscribe(core.EventSubscriptionAdded, p.subAddedID)
			}
			if p.subSuppID != 0 {
				unsubBus.Unsubscribe(core.EventSupplementTriggered, p.subSuppID)
			}
		}
		p.subAddedID = 0
		p.subSuppID = 0
		p.subBus = nil
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}

	p.runCtx, p.cancel = context.WithCancel(context.Background())
	p.enabled = true
	p.subBus = bus

	if bus != nil {
		p.subAddedID = bus.Subscribe(core.EventSubscriptionAdded, func(ev core.Event) {
			p.dispatchMapEvent(ev)
		})
		p.subSuppID = bus.Subscribe(core.EventSupplementTriggered, func(ev core.Event) {
			p.dispatchMapEvent(ev)
		})
		p.initialized = true
	} else {
		p.initialized = false
	}

	log.Println("🔌 [插件] 元数据映射器已启用")
	return nil
}

// Stop 停用元数据映射插件，注销事件并等待在途映射任务结束
func (p *MetadataMappingPlugin) Stop(bus core.EventBus) error {
	p.mu.Lock()
	wasEnabled := p.enabled
	p.enabled = false
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.runCtx = nil

	unsubBus := bus
	if unsubBus == nil {
		unsubBus = p.subBus
	}
	if p.initialized && unsubBus != nil {
		if p.subAddedID != 0 {
			unsubBus.Unsubscribe(core.EventSubscriptionAdded, p.subAddedID)
		}
		if p.subSuppID != 0 {
			unsubBus.Unsubscribe(core.EventSupplementTriggered, p.subSuppID)
		}
	}
	p.subAddedID = 0
	p.subSuppID = 0
	p.subBus = nil
	p.initialized = false
	p.mu.Unlock()

	p.wg.Wait()
	if wasEnabled {
		log.Println("🔌 [插件] 元数据映射器已停用，已注销事件监听")
	}
	return nil
}

func (p *MetadataMappingPlugin) dispatchMapEvent(ev core.Event) {
	subID := parseSubID(ev)
	if subID == 0 {
		return
	}

	p.mu.RLock()
	if !p.enabled || p.runCtx == nil || p.runCtx.Err() != nil {
		p.mu.RUnlock()
		return
	}
	runCtx := p.runCtx
	p.wg.Add(1)
	p.mu.RUnlock()

	go func() {
		defer p.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("⚠️ [元数据映射] 后台任务异常崩溃捕获 (subID=%d): %v", subID, r)
			}
		}()
		p.mapOne(runCtx, subID)
	}()
}

func parseSubID(ev core.Event) uint {
	if ev.Payload == nil {
		return 0
	}
	switch v := ev.Payload["subscription_id"].(type) {
	case uint:
		return v
	case int:
		if v > 0 {
			return uint(v)
		}
	case int64:
		if v > 0 {
			return uint(v)
		}
	case uint64:
		return uint(v)
	case float64:
		if v > 0 {
			return uint(v)
		}
	}
	return 0
}

func (p *MetadataMappingPlugin) isActive(ctx context.Context) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	p.mu.RLock()
	active := p.enabled
	p.mu.RUnlock()
	return active
}

func (p *MetadataMappingPlugin) mapOne(parentCtx context.Context, subID uint) {
	if !p.isActive(parentCtx) || database.DB == nil {
		return
	}

	var sub database.Subscription
	if err := database.DB.First(&sub, subID).Error; err != nil || sub.BangumiID == "" {
		return
	}
	if sub.TMDBID != "" && sub.IMDBID != "" {
		return
	}

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
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	tmdb, imdb := mapper.MapBangumiToExternal(ctx, sub.BangumiID, sub.TitleCN, sub.Year)

	if !p.isActive(parentCtx) {
		return
	}

	updates := map[string]interface{}{}
	if tmdb != "" {
		updates["tmdb_id"] = tmdb
		// 若通过 TMDB 获取到 Collection 系列信息且当前未设置 SeriesID，补充关联
		if tmdbProvider != nil && sub.SeriesID == "" {
			tmdbIDClean := strings.TrimPrefix(tmdb, "tv/")
			if detail, err := tmdbProvider.GetAnime(ctx, tmdbIDClean); err == nil && detail.SeriesID != "" {
				updates["series_id"] = detail.SeriesID
			}
		}
	}
	if imdb != "" {
		updates["imdb_id"] = imdb
	}
	if len(updates) > 0 && p.isActive(parentCtx) {
		if err := database.DB.Model(&sub).Updates(updates).Error; err != nil {
			log.Printf("⚠️ [插件] 保存映射结果失败 [%s]: %v", sub.TitleCN, err)
			return
		}
		log.Printf("✅ [插件] 关联成功 [%s]: %v", sub.TitleCN, updates)
	}
}
