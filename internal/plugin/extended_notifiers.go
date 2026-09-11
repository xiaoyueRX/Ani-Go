package plugin

import (
	"log"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// NotifyController 用于控制核心通知中心启停与重新加载
type NotifyController interface {
	SetEnabled(enabled bool)
	IsEnabled() bool
	Reload()
}

// ExtendedNotifiersPlugin 消息通知推送插件（统一管理全渠道消息推送）
// 作为 v2.NotifyManager 的插件化控制中枢，统一纳管 Telegram, 企业微信, 飞书, 钉钉, QQ (OneBot),
// 以及 Bark, Server酱, Discord, Slack, Gotify, Ntfy, Pushover, Email SMTP, Matrix, LINE, WhatsApp, Signal 等全部 16+ 种通道
type ExtendedNotifiersPlugin struct {
	mu         sync.RWMutex
	enabled    bool
	controller NotifyController
}

// DefaultExtendedNotifiersPlugin 共享单例
var DefaultExtendedNotifiersPlugin = &ExtendedNotifiersPlugin{}

// SetController 绑定底层 NotifyManager 控制接口
func (p *ExtendedNotifiersPlugin) SetController(c NotifyController) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.controller = c
	if c != nil {
		c.SetEnabled(p.enabled)
	}
}

func (p *ExtendedNotifiersPlugin) GetInfo() PluginInfo {
	return PluginInfo{
		ID:          "extended-notifiers",
		Name:        "消息通知推送",
		Description: "全协议消息通知推送中心，基于 v2.NotifyManager 统一纳管 Telegram、企业微信、飞书、钉钉、QQ (OneBot)、Bark、Server酱、Discord、Slack、Gotify、Ntfy、Pushover、Email SMTP、Matrix、LINE、WhatsApp、Signal 等 16+ 渠道消息提醒与事件推送。",
		Version:     "2.0.0",
		Author:      "xiaoyue",
		AuthorURL:   "https://github.com/xiaoyueRX",
		Icon:        "Bell",
		IsBuiltIn:   true,
		Type:        "builtin",
		Events: []string{
			core.EventDownloadStarted,
			core.EventDownloadCompleted,
			core.EventFileOrganized,
			core.EventSupplementCompleted,
			core.EventDownloadFailed,
		},
	}
}

// Init 启动通知插件
func (p *ExtendedNotifiersPlugin) Init(bus core.EventBus, ctx core.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.enabled = true
	if p.controller != nil {
		p.controller.SetEnabled(true)
		p.controller.Reload()
	}
	log.Println("🔌 [插件] 消息通知推送插件已启用，已激活 v2.NotifyManager 全渠道通知中心")
	return nil
}

// Stop 停用通知插件
func (p *ExtendedNotifiersPlugin) Stop(bus core.EventBus) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.enabled = false
	if p.controller != nil {
		p.controller.SetEnabled(false)
	}
	log.Println("🔌 [插件] 消息通知推送插件已停用，已休眠通知中心（所有渠道消息推送已暂停）")
	return nil
}
