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
		ID:          "geek-naming",
		Name:        "极客智能文件命名",
		Description: "在重命名与整理番剧文件时，注入智能极客前缀 [Ani-Go] 标记与标准化路径。",
		Version:     "1.0.0",
		Author:      "xiaoyue",
		AuthorURL:   "https://github.com/xiaoyueRX",
		Icon:        "Sparkles",
		IsBuiltIn:   true,
		Type:        "builtin",
		Events:      []string{core.EventFileOrganized},
	}
}

func (p *GeekNamingPlugin) Init(bus core.EventBus, ctx core.Context) error {
	log.Println("🔌 [插件] 极客智能文件命名插件已启动")
	return nil
}

// InitGeekNaming 注册极客命名插件
func InitGeekNaming(mgr *core.WaterfallHookManager) {
	mgr.RegisterNamingHook(core.PriorityHigh, "GeekNaming", func(ctx context.Context, input interface{}) (interface{}, error) {
		in := input.(core.NamingHookInput)
		newPath := "[Ani-Go] " + in.RenderedPath
		log.Printf("🧪 [插件魔法] 正在重命名: %s -> %s", in.RenderedPath, newPath)
		return core.NamingHookOutput{
			RenderedPath: newPath,
		}, nil
	})
}
