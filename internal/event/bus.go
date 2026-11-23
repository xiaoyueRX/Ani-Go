// Package event 为每个订阅单独排队，异步投递核心业务事件。
package event

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

const queueCapacity = 256
const defaultHandlerTimeout = 5 * time.Second

var subCounter atomic.Uint64

type sub struct {
	id      core.SubscriptionID
	fn      core.EventHandler
	running bool
	pending chan core.Event
}

type Bus struct {
	mu             sync.Mutex
	handlers       map[string][]*sub
	workers        int
	idle           chan struct{}
	closed         bool
	handlerTimeout time.Duration
	warnings       *time.Timer
	fullDeliveries uint64
	latePublishes  uint64
}

// PublishResult 按订阅报告接纳和拒绝的投递数，Closed 表示发布时总线已经关闭。
type PublishResult struct {
	Accepted, Full int
	Closed         bool
}

var _ core.EventBus = (*Bus)(nil)

func New() *Bus {
	return &Bus{handlers: make(map[string][]*sub), handlerTimeout: defaultHandlerTimeout}
}

func (b *Bus) SetHandlerTimeout(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	b.mu.Lock()
	b.handlerTimeout = timeout
	b.mu.Unlock()
}

// Publish 非阻塞；饱和与关闭后的拒绝按秒汇总记录，不记录负载。
func (b *Bus) Publish(event core.Event) {
	result := b.TryPublish(event)
	if result.Full == 0 && !result.Closed {
		return
	}
	b.mu.Lock()
	b.fullDeliveries += uint64(result.Full)
	if result.Closed {
		b.latePublishes++
	}
	if b.warnings == nil {
		b.warnings = time.AfterFunc(time.Second, func() {
			b.mu.Lock()
			full, late := b.fullDeliveries, b.latePublishes
			b.fullDeliveries, b.latePublishes, b.warnings = 0, 0, nil
			b.mu.Unlock()
			log.Printf("事件总线拒绝投递：队满 %d 次，关闭后发布 %d 次", full, late)
		})
	}
	b.mu.Unlock()
}

func (b *Bus) TryPublish(event core.Event) PublishResult {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return PublishResult{Closed: true}
	}
	var result PublishResult
	for _, s := range b.handlers[event.Type] {
		if !s.running {
			if b.workers == 0 {
				b.idle = make(chan struct{})
			}
			s.running = true
			b.workers++
			go b.drain(s, event, b.handlerTimeout)
			result.Accepted++
			continue
		}
		if s.pending == nil {
			s.pending = make(chan core.Event, queueCapacity)
		}
		select {
		case s.pending <- event:
			result.Accepted++
		default:
			result.Full++
		}
	}
	return result
}

func (b *Bus) drain(s *sub, event core.Event, timeout time.Duration) {
	for {
		b.runHandler(s, event, timeout)
		b.mu.Lock()
		select {
		case event = <-s.pending:
			timeout = b.handlerTimeout
			b.mu.Unlock()
		default:
			s.running = false
			s.pending = nil
			b.workers--
			if b.workers == 0 {
				close(b.idle)
				b.idle = nil
			}
			b.mu.Unlock()
			return
		}
	}
}

func (b *Bus) runHandler(s *sub, event core.Event, timeout time.Duration) {
	timer := time.AfterFunc(timeout, func() {
		log.Printf("事件处理仍在运行（%s，订阅 %d，已超过 %v）", event.Type, s.id, timeout)
	})
	defer timer.Stop()
	defer func() {
		if issue := recover(); issue != nil {
			log.Printf("事件回调异常（%s，订阅 %d）", event.Type, s.id)
		}
	}()
	s.fn(event)
}

func (b *Bus) Subscribe(eventType string, handler core.EventHandler) core.SubscriptionID {
	if handler == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0
	}
	id := core.SubscriptionID(subCounter.Add(1))
	b.handlers[eventType] = append(b.handlers[eventType], &sub{id: id, fn: handler})
	return id
}

func (b *Bus) Unsubscribe(eventType string, id core.SubscriptionID) {
	if id == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	entries := b.handlers[eventType]
	for i, s := range entries {
		if s.id != id {
			continue
		}
		copy(entries[i:], entries[i+1:])
		entries[len(entries)-1] = nil
		if len(entries) == 1 {
			delete(b.handlers, eventType)
		} else {
			b.handlers[eventType] = entries[:len(entries)-1]
		}
		return
	}
}

func (b *Bus) Wait() {
	b.mu.Lock()
	idle := b.idle
	b.mu.Unlock()
	if idle != nil {
		<-idle
	}
}

// Close 关闭入口，等待已接纳的回调真实退出。调用者超时后可以再次等待。
func (b *Bus) Close(ctx context.Context) error {
	b.mu.Lock()
	b.closed = true
	clear(b.handlers)
	idle := b.idle
	b.mu.Unlock()
	if idle == nil {
		return nil
	}
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
