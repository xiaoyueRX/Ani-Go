package plugin

import (
	"log"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type YucSchedulePlugin struct {
	mu sync.Mutex
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

func (p *YucSchedulePlugin) Init(bus core.EventBus, ctx core.Context) error {
	log.Println("🔌 [插件] 長門番堂 (yuc.wiki) 时间表插件已加载")
	return nil
}
