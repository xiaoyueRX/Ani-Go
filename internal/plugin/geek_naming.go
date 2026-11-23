package plugin

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

var (
	// DefaultGeekNamingPlugin 媒体库规范重命名插件共享单例
	DefaultGeekNamingPlugin = &GeekNamingPlugin{}
)

func init() {
	DefaultGeekNamingPlugin.enabled.Store(true)
}

// GeekNamingPlugin 媒体库规范重命名插件
type GeekNamingPlugin struct {
	mu         sync.Mutex
	enabled    atomic.Bool
	activePctx PluginContext
}

func (p *GeekNamingPlugin) GetInfo() PluginInfo {
	return PluginInfo{
		ID:          "standard-naming",
		Name:        "媒体库规范重命名",
		Description: "在番剧入库与文件整理时，遵循 Plex/Emby/Jellyfin 刮削规范，自动格式化文件名与季集目录结构。",
		Version:     "1.0.0",
		Author:      "xiaoyue",
		AuthorURL:   "https://github.com/xiaoyueRX",
		Icon:        "FolderCheck",
		IsBuiltIn:   true,
		Type:        "builtin",
		Events:      []string{core.EventFileOrganized},
	}
}

// Init 启用媒体库规范命名插件（集成 PluginContext 与 OnDispose 销毁监听）
func (p *GeekNamingPlugin) Init(bus core.EventBus, ctx core.Context) error {
	p.mu.Lock()
	oldCtx := p.activePctx
	p.activePctx = nil
	p.mu.Unlock()

	if oldCtx != nil {
		oldCtx.Dispose()
	}

	pctx, ok := ctx.(PluginContext)
	if !ok {
		pctx = NewPluginContext("standard-naming", bus, ctx)
	}

	p.mu.Lock()
	p.activePctx = pctx
	p.enabled.Store(true)
	p.mu.Unlock()

	pctx.OnDispose(func() {
		p.enabled.Store(false)
	})

	pctx.Logf("媒体库规范重命名插件已启用")
	return nil
}

// Stop 停用媒体库规范命名插件，触发上下文 Dispose
func (p *GeekNamingPlugin) Stop(bus core.EventBus) error {
	p.mu.Lock()
	p.enabled.Store(false)
	pctx := p.activePctx
	p.activePctx = nil
	p.mu.Unlock()

	if pctx != nil {
		pctx.Dispose()
	}
	log.Println("🔌 [插件] 媒体库规范重命名插件已停用，钩子将跳过规范化处理")
	return nil
}

// InitGeekNaming 注册规范命名钩子（重复调用时先清理同名旧钩子以保证幂等）
func InitGeekNaming(mgr *core.WaterfallHookManager) {
	if mgr == nil {
		return
	}
	mgr.UnregisterHooksByName("StandardNaming")
	mgr.RegisterNamingHook(core.PriorityHigh, "StandardNaming", func(ctx context.Context, input interface{}) (interface{}, error) {
		in, ok := input.(core.NamingHookInput)
		if !ok {
			return nil, fmt.Errorf("规范命名钩子收到无效输入类型: %T", input)
		}
		if !DefaultGeekNamingPlugin.enabled.Load() {
			return core.NamingHookOutput{
				RenderedPath: in.RenderedPath,
			}, nil
		}
		sanitized := core.SanitizePathForMediaServer(in.RenderedPath)
		if sanitized != in.RenderedPath {
			log.Printf("📁 [规范命名] 优化文件路径: %s -> %s", in.RenderedPath, sanitized)
		}
		return core.NamingHookOutput{
			RenderedPath: sanitized,
		}, nil
	})
}
