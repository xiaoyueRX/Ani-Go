package notifier

import (
	"context"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// Manager 封装了整个通知系统的生命周期管理
type Manager struct {
	Dispatcher      *Dispatcher
	TemplateManager *TemplateManager
	WSNotifier      *WSNotifier
}

// Setup 初始化整个通知系统
func Setup(ctx context.Context, config map[string]interface{}) *Manager {
	tm := NewTemplateManager()
	
	var notifiers []core.Notifier

	// 1. Websocket (默认启动)
	ws := NewWSNotifier()
	notifiers = append(notifiers, ws)

	// 2. Telegram (可选配置)
	if token, ok := config["telegram_token"].(string); ok && token != "" {
		chatID := config["telegram_chat_id"].(string)
		notifiers = append(notifiers, NewTelegramNotifier(token, chatID))
	}

	// 3. Discord (可选配置)
	if webhook, ok := config["discord_webhook"].(string); ok && webhook != "" {
		notifiers = append(notifiers, NewDiscordNotifier(webhook))
	}

	// 初始化分发器
	dispatcher := NewDispatcher(tm, notifiers...)
	dispatcher.Start(ctx)

	return &Manager{
		Dispatcher:      dispatcher,
		TemplateManager: tm,
		WSNotifier:      ws,
	}
}

// Notify 快捷发送入口
func (m *Manager) Notify(title, templateID string, data map[string]interface{}) {
	m.Dispatcher.Publish(&Message{
		Title:      title,
		TemplateID: templateID,
		Data:       data,
	})
}

// NotifyRaw 直接发送原始文字
func (m *Manager) NotifyRaw(title, content string) {
	m.Dispatcher.Publish(&Message{
		Title:   title,
		Content: content,
	})
}

// HandleEvent 适配 core.EventBus
func (m *Manager) HandleEvent(event core.Event) {
	// 将 EventBus 的事件转化为通知消息
	m.Dispatcher.Publish(&Message{
		Title:      "系统通知",
		TemplateID: event.Type,
		Data:       event.Payload,
	})
}
