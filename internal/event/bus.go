// Package event 实现事件总线（发布/订阅模式）
// 用于各模块之间的松耦合适信
package event

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// SubscriptionID 全局递增计数器
var subCounter atomic.Uint64

// sub 订阅条目，包含唯一 ID 和处理函数
type sub struct {
	id core.SubscriptionID
	fn core.EventHandler
}

// Bus 事件总线实现
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]sub
}

// New 创建事件总线实例
func New() *Bus {
	return &Bus{
		handlers: make(map[string][]sub),
	}
}

// Publish 发布一个事件，通知所有订阅了该事件类型的处理器
func (b *Bus) Publish(event core.Event) {
	b.mu.RLock()
	handlers := b.handlers[event.Type]
	b.mu.RUnlock()

	for _, h := range handlers {
		go func(s sub) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("⚠️  EventBus 处理器 panic (事件: %s): %v", event.Type, r)
				}
			}()

			// 带超时的处理器执行，防止 goroutine 泄漏
			done := make(chan struct{})
			go func() {
				defer close(done)
				s.fn(event)
			}()

			select {
			case <-done:
				// 正常完成
			case <-time.After(5 * time.Second):
				log.Printf("⚠️  EventBus 处理器超时 (事件: %s, subID: %d)", event.Type, s.id)
			}
		}(h)
	}
}

// Subscribe 订阅指定事件类型，返回唯一 SubscriptionID
func (b *Bus) Subscribe(eventType string, handler core.EventHandler) core.SubscriptionID {
	id := core.SubscriptionID(subCounter.Add(1))
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], sub{id: id, fn: handler})
	return id
}

// Unsubscribe 通过 SubscriptionID 取消订阅
func (b *Bus) Unsubscribe(eventType string, id core.SubscriptionID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlers := b.handlers[eventType]
	for i, h := range handlers {
		if h.id == id {
			b.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			return
		}
	}
}
