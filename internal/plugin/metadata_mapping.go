package plugin

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/metadata"
	"gorm.io/gorm"
)

// MetadataMappingPlugin 元数据映射器插件，自动关联 BangumiID 与 TMDB / IMDB
type MetadataMappingPlugin struct {
	mu         sync.Mutex
	enabled    atomic.Bool
	activePctx PluginContext
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

// Init 启用元数据映射插件（基于 PluginContext 与 OnDispose 销毁模式，幂等支持重复调用）
func (p *MetadataMappingPlugin) Init(bus core.EventBus, ctx core.Context) error {
	p.mu.Lock()
	oldCtx := p.activePctx
	p.activePctx = nil
	p.mu.Unlock()

	if oldCtx != nil {
		oldCtx.Dispose()
	}

	pctx, ok := ctx.(PluginContext)
	if !ok {
		pctx = NewPluginContext("metadata-mapping", bus, ctx)
	}

	p.mu.Lock()
	p.activePctx = pctx
	p.enabled.Store(true)
	p.mu.Unlock()

	// 注册生命周期销毁回调
	pctx.OnDispose(func() {
		p.enabled.Store(false)
	})

	// 订阅事件总线并在插件停用时自动注销
	if bus != nil {
		pctx.Subscribe(core.EventSubscriptionAdded, func(ev core.Event) {
			p.dispatchMapEvent(ev, pctx)
		})
		pctx.Subscribe(core.EventSupplementTriggered, func(ev core.Event) {
			p.dispatchMapEvent(ev, pctx)
		})
	}

	pctx.Logf("元数据映射器已启用")
	return nil
}

// Stop 停用元数据映射插件，触发上下文 Dispose 释放所有监听与在途任务
func (p *MetadataMappingPlugin) Stop(bus core.EventBus) error {
	p.mu.Lock()
	wasEnabled := p.enabled.Swap(false)
	pctx := p.activePctx
	p.activePctx = nil
	p.mu.Unlock()

	if pctx != nil {
		pctx.Dispose()
	}

	if wasEnabled {
		log.Println("🔌 [插件] 元数据映射器已停用，已注销事件监听与在途任务")
	}
	return nil
}

func (p *MetadataMappingPlugin) dispatchMapEvent(ev core.Event, pctx PluginContext) {
	subID := parseSubID(ev)
	if subID == 0 {
		return
	}

	if !p.enabled.Load() || pctx == nil || pctx.Context().Err() != nil {
		return
	}

	// 派生受管后台协程
	pctx.Go(func(taskCtx context.Context) {
		p.mapOne(taskCtx, subID, pctx)
	})
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
	return p.enabled.Load()
}

func (p *MetadataMappingPlugin) mapOne(parentCtx context.Context, subID uint, pctx PluginContext) {
	if !p.isActive(parentCtx) {
		return
	}

	// 解析数据库句柄
	var gormDB *gorm.DB
	if pctx != nil {
		if dbVal, ok := pctx.GetDB().(*gorm.DB); ok && dbVal != nil {
			gormDB = dbVal
		}
	}
	if gormDB == nil {
		gormDB = database.DB
	}
	if gormDB == nil {
		return
	}

	var sub database.Subscription
	if err := gormDB.First(&sub, subID).Error; err != nil || sub.BangumiID == "" {
		return
	}
	if sub.TMDBID != "" && sub.IMDBID != "" {
		return
	}

	log.Printf("🔗 [插件] 正在寻找 [%s] 的外部 ID 映射...", sub.TitleCN)

	// 内部创建映射器：若配置了 TMDB_API_KEY 则注入 TMDBProvider 实现自动搜索兜底
	var tmdbProvider *metadata.TMDBProvider
	tmdbKey := ""
	var s database.Setting
	if err := gormDB.Where("key = ?", "TMDB_API_KEY").First(&s).Error; err == nil {
		tmdbKey = strings.TrimSpace(s.Value)
	}
	if tmdbKey == "" {
		tmdbKey = strings.TrimSpace(os.Getenv("TMDB_API_KEY"))
	}
	if tmdbKey != "" {
		lang := "zh-CN"
		var mirrors []string
		var mirrorSetting database.Setting
		if err := gormDB.Where("key = ?", "TMDB_MIRROR_DOMAINS").First(&mirrorSetting).Error; err == nil && mirrorSetting.Value != "" {
			for _, m := range strings.Split(mirrorSetting.Value, ",") {
				if tr := strings.TrimSpace(m); tr != "" {
					mirrors = append(mirrors, tr)
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
		if err := gormDB.Model(&sub).Updates(updates).Error; err != nil {
			log.Printf("⚠️ [插件] 保存映射结果失败 [%s]: %v", sub.TitleCN, err)
			return
		}
		log.Printf("✅ [插件] 关联成功 [%s]: %v", sub.TitleCN, updates)
	}
}
