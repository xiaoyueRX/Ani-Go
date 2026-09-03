package plugin

import (
	"context"
	"log"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type GeekNamingPlugin struct {
	mu sync.Mutex
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

func (p *GeekNamingPlugin) Init(bus core.EventBus, ctx core.Context) error {
	log.Println("🔌 [插件] 媒体库规范重命名插件已加载")
	return nil
}

// InitGeekNaming 注册规范命名插件
func InitGeekNaming(mgr *core.WaterfallHookManager) {
	mgr.RegisterNamingHook(core.PriorityHigh, "StandardNaming", func(ctx context.Context, input interface{}) (interface{}, error) {
		in := input.(core.NamingHookInput)
		log.Printf("📁 [规范命名] 整理文件路径: %s", in.RenderedPath)
		return core.NamingHookOutput{
			RenderedPath: in.RenderedPath,
		}, nil
	})
}
