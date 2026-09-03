package plugin

import (
	"context"
	"log"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

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
