// Package event 实现事件总线（发布/订阅模式）
// 用于各模块之间的解耦异步通知，具备协程上限保护、零泄漏计时器与崩溃隔离
package event

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// defaultMaxWorkers 默认并发事件处理协程上限，防止海量突发事件触发协程风暴
const defaultMaxWorkers = 128

// defaultHandlerTimeout 默认单个事件处理器最长执行时间
const defaultHandlerTimeout = 5 * time.Second

// SubscriptionID 全局递增计数器
var subCounter atomic.Uint64

// sub 订阅条目，包含唯一 ID 和处理函数
type sub struct {
	id core.SubscriptionID
	fn core.EventHandler
}

// Bus 事件总线实现
type Bus struct {
	mu             sync.RWMutex
	handlers       map[string][]sub
	workerSem      chan struct{}
	handlerTimeout time.Duration
	activeWg       sync.WaitGroup
	closed         atomic.Bool
}

var _ core.EventBus = (*Bus)(nil)

// New 创建事件总线实例
func New() *Bus {
	return &Bus{
		handlers:       make(map[string][]sub),
		workerSem:      make(chan struct{}, defaultMaxWorkers),
		handlerTimeout: defaultHandlerTimeout,
	}
}

// SetHandlerTimeout 设置单个事件处理器的超时时间（主要用于单元测试与调试）
func (b *Bus) SetHandlerTimeout(timeout time.Duration) {
	if timeout > 0 {
		b.handlerTimeout = timeout
	}
}

// Publish 发布一个事件，通知所有订阅了该事件类型的处理器
// 保证非阻塞返回、零丢失事件与异常隔离
func (b *Bus) Publish(event core.Event) {
	if b.closed.Load() {
		return
	}

	b.mu.RLock()
	subs := b.handlers[event.Type]
	if len(subs) == 0 {
		b.mu.RUnlock()
		return
	}
	// 深拷贝切片快照，彻底根绝迭代过程中并发 Unsubscribe 导致的数据竞争与切片截断
	handlers := make([]sub, len(subs))
	copy(handlers, subs)
	b.mu.RUnlock()

	for _, h := range handlers {
		s := h
		select {
		case b.workerSem <- struct{}{}:
			go func() {
				defer func() { <-b.workerSem }()
				b.runHandler(s, event)
			}()
		default:
			// 信号量已满时的弹性降级：独立启动协程执行，确保 100% 零丢事件
			go b.runHandler(s, event)
		}
	}
}

// runHandler 执行单个订阅者的处理逻辑，提供 Panic 恢复与安全定时器回收
func (b *Bus) runHandler(s sub, event core.Event) {
	b.activeWg.Add(1)
	defer b.activeWg.Done()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠️  EventBus 处理器 panic (事件: %s, subID: %d): %v", event.Type, s.id, r)
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				log.Printf("⚠️  EventBus 处理器内部 panic (事件: %s, subID: %d): %v", event.Type, s.id, r)
			}
		}()
		s.fn(event)
	}()

	timer := time.NewTimer(b.handlerTimeout)
	select {
	case <-done:
		// 正常完成，显式释放并排空 Timer，防止海量事件下 time.Timer 残留导致堆内存溢出
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	case <-timer.C:
		log.Printf("⚠️  EventBus 处理器执行超时 (事件: %s, subID: %d, 阈值: %v)", event.Type, s.id, b.handlerTimeout)
	}
}

// Subscribe 订阅指定事件类型，返回唯一 SubscriptionID
func (b *Bus) Subscribe(eventType string, handler core.EventHandler) core.SubscriptionID {
	if handler == nil {
		return 0
	}
	id := core.SubscriptionID(subCounter.Add(1))
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], sub{id: id, fn: handler})
	return id
}

// Unsubscribe 通过 SubscriptionID 取消订阅
func (b *Bus) Unsubscribe(eventType string, id core.SubscriptionID) {
	if id == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	handlers := b.handlers[eventType]
	for i, h := range handlers {
		if h.id == id {
			// 分配新切片以防正在迭代旧切片底层的并发 Publish 发生脏读
			newHandlers := make([]sub, 0, len(handlers)-1)
			newHandlers = append(newHandlers, handlers[:i]...)
			newHandlers = append(newHandlers, handlers[i+1:]...)
			if len(newHandlers) == 0 {
				delete(b.handlers, eventType)
			} else {
				b.handlers[eventType] = newHandlers
			}
			return
		}
	}
}

// Wait 等待当前所有正在执行的事件处理协程完成
func (b *Bus) Wait() {
	b.activeWg.Wait()
}

// Close 安全关闭事件总线并等待在途任务完成或超时
func (b *Bus) Close(ctx context.Context) error {
	b.closed.Store(true)
	done := make(chan struct{})
	go func() {
		b.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
