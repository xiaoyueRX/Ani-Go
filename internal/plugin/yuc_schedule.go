package plugin

import (
	"log"
	"sync"
	"sync/atomic"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// YucSchedulePlugin 長門番堂 (yuc.wiki) 时间表插件
type YucSchedulePlugin struct {
	mu         sync.Mutex
	enabled    atomic.Bool
	activePctx PluginContext
}

func (p *YucSchedulePlugin) GetInfo() PluginInfo {
	return PluginInfo{
		ID:          "yuc_schedule",
		Name:        "長門番堂 (yuc.wiki) 时间表",
		Description: "启用後将在时间表数据源中增加長門番堂（yuc.wiki）新番时间表支持。默认关闭，开启后可在时间表页面切换至 Yuc.wiki 数据源。",
		Version:     "1.0.0",
		Author:      "xiaoyue",
		AuthorURL:   "https://github.com/xiaoyueRX",
		Icon:        "Calendar",
		IsBuiltIn:   true,
		Type:        "builtin",
		Events:      []string{},
	}
}

// Init 启用时间表插件（基于 PluginContext 与 OnDispose 销毁管理）
func (p *YucSchedulePlugin) Init(bus core.EventBus, ctx core.Context) error {
	p.mu.Lock()
	oldCtx := p.activePctx
	p.activePctx = nil
	p.mu.Unlock()

	if oldCtx != nil {
		oldCtx.Dispose()
	}

	pctx, ok := ctx.(PluginContext)
	if !ok {
		pctx = NewPluginContext("yuc_schedule", bus, ctx)
	}

	p.mu.Lock()
	p.activePctx = pctx
	p.enabled.Store(true)
	p.mu.Unlock()

	pctx.OnDispose(func() {
		p.enabled.Store(false)
	})

	pctx.Logf("長門番堂 (yuc.wiki) 时间表插件已加载并启用")
	return nil
}

// Stop 停用时间表插件，触发上下文 Dispose
func (p *YucSchedulePlugin) Stop(bus core.EventBus) error {
	p.mu.Lock()
	p.enabled.Store(false)
	pctx := p.activePctx
	p.activePctx = nil
	p.mu.Unlock()

	if pctx != nil {
		pctx.Dispose()
	}
	log.Println("🔌 [插件] 長門番堂 (yuc.wiki) 时间表插件已停用")
	return nil
}
