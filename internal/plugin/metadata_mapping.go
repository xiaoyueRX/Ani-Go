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
	inFlightMu sync.Mutex
	inFlight   map[uint]struct{}
}

func (*MetadataMappingPlugin) Dependencies() []core.ServiceType {
	return []core.ServiceType{core.ServiceEventBus}
}

func (p *MetadataMappingPlugin) GetInfo() PluginInfo {
	return PluginInfo{
		ID:          "metadata-mapping",
		Name:        "元数据映射器",
		Description: "自动关联 BangumiID 到 TMDB/IMDB，打通全平台追番信息。",
		Version:     "1.1.0",
		Author:      "xiaoyueRX",
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

func (p *MetadataMappingPlugin) acquireInFlight(subID uint) bool {
	p.inFlightMu.Lock()
	defer p.inFlightMu.Unlock()
	if p.inFlight == nil {
		p.inFlight = make(map[uint]struct{})
	}
	if _, exists := p.inFlight[subID]; exists {
		return false
	}
	p.inFlight[subID] = struct{}{}
	return true
}

func (p *MetadataMappingPlugin) releaseInFlight(subID uint) {
	p.inFlightMu.Lock()
	delete(p.inFlight, subID)
	p.inFlightMu.Unlock()
}

func resolveTMDBConfig(gormDB *gorm.DB) (apiKey, language string, mirrors []string) {
	readValue := func(key string) string {
		if gormDB != nil {
			var s database.Setting
			if err := gormDB.Where("key = ?", key).First(&s).Error; err == nil {
				if v := strings.TrimSpace(s.Value); v != "" {
					return v
				}
			}
		}
		return strings.TrimSpace(os.Getenv(key))
	}

	apiKey = readValue("TMDB_API_KEY")
	language = readValue("TMDB_LANGUAGE")
	if language == "" {
		language = "zh-CN"
	}
	if rawMirrors := readValue("TMDB_MIRROR_DOMAINS"); rawMirrors != "" {
		for _, m := range strings.Split(rawMirrors, ",") {
			if tr := strings.TrimSpace(m); tr != "" {
				mirrors = append(mirrors, tr)
			}
		}
	}
	return apiKey, language, mirrors
}

func (p *MetadataMappingPlugin) mapOne(parentCtx context.Context, subID uint, pctx PluginContext) {
	if !p.isActive(parentCtx) || !p.acquireInFlight(subID) {
		return
	}
	defer p.releaseInFlight(subID)

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
	if err := gormDB.First(&sub, subID).Error; err != nil || sub.BGMSubjectID() == "" {
		return
	}
	if sub.TMDBID != "" && sub.IMDBID != "" && sub.SeriesID != "" {
		return
	}

	// 内部创建映射器：若配置了 TMDB_API_KEY 则注入 TMDBProvider 实现自动搜索兜底
	var tmdbProvider *metadata.TMDBProvider
	tmdbKey, lang, mirrors := resolveTMDBConfig(gormDB)
	if tmdbKey != "" {
		tmdbProvider = metadata.NewTMDBProvider(tmdbKey, lang, mirrors)
	}

	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()

	var tmdb, imdb string
	if sub.TMDBID == "" || sub.IMDBID == "" {
		log.Printf("🔗 [插件] 正在寻找 [%s] 的外部 ID 映射...", sub.TitleCN)
		mapper := metadata.NewMapper(tmdbProvider)
		tmdb, imdb = mapper.MapBangumiToExternal(ctx, sub.BGMSubjectID(), sub.TitleCN, sub.Year)
	}

	if !p.isActive(parentCtx) {
		return
	}

	updates := map[string]interface{}{}
	if tmdb != "" && sub.TMDBID == "" {
		updates["tmdb_id"] = tmdb
	}
	if imdb != "" && sub.IMDBID == "" {
		updates["imdb_id"] = imdb
	}
	// 若通过 TMDB 获取到 Collection 系列信息且当前未设置 SeriesID，补充关联（兼容 tv/ 与 movie/ 前缀）
	effectiveTMDB := strings.TrimSpace(sub.TMDBID)
	if effectiveTMDB == "" {
		effectiveTMDB = tmdb
	}
	if effectiveTMDB != "" && tmdbProvider != nil && sub.SeriesID == "" {
		if detail, err := tmdbProvider.GetAnime(ctx, effectiveTMDB); err == nil && detail.SeriesID != "" {
			updates["series_id"] = detail.SeriesID
		}
	}
	if len(updates) > 0 && p.isActive(parentCtx) {
		if err := gormDB.Model(&sub).Updates(updates).Error; err != nil {
			log.Printf("⚠️ [插件] 保存映射结果失败 [%s]: %v", sub.TitleCN, err)
			return
		}
		log.Printf("✅ [插件] 关联成功 [%s]: %v", sub.TitleCN, updates)
	}
}
