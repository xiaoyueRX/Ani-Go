// Package event 实现事件总线（发布/订阅模式）
// 用于各模块之间的松耦合适信
package event

import (
	"log"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// Bus 事件总线实现
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]core.EventHandler
}

// New 创建事件总线实例
func New() *Bus {
	return &Bus{
		handlers: make(map[string][]core.EventHandler),
	}
}

// Publish 发布一个事件，通知所有订阅了该事件类型的处理器
func (b *Bus) Publish(event core.Event) {
	b.mu.RLock()
	handlers := b.handlers[event.Type]
	b.mu.RUnlock()

	for _, h := range handlers {
		go func(handler core.EventHandler) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("⚠️  EventBus 处理器 panic (事件: %s): %v", event.Type, r)
				}
			}()
			handler(event)
		}(h)
	}
}

// Subscribe 订阅指定事件类型
func (b *Bus) Subscribe(eventType string, handler core.EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Unsubscribe 取消订阅指定事件类型
func (b *Bus) Unsubscribe(eventType string, handler core.EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlers := b.handlers[eventType]
	for i, h := range handlers {
		// 通过函数指针地址比较来移除
		if &h == &handler {
			b.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			return
		}
	}
}
