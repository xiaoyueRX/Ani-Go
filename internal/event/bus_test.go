package event

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestBusPublishSubscribe(t *testing.T) {
	bus := New()

	var wg sync.WaitGroup
	wg.Add(1)

	received := false
	bus.Subscribe("test.event", func(event core.Event) {
		if event.Type == "test.event" {
			received = true
		}
		wg.Done()
	})

	bus.Publish(core.Event{Type: "test.event"})
	wg.Wait()

	if !received {
		t.Error("事件处理器未被调用")
	}
}

func TestBusMultipleHandlers(t *testing.T) {
	bus := New()

	var wg sync.WaitGroup
	wg.Add(2)

	count := 0
	var mu sync.Mutex

	handler := func(event core.Event) {
		mu.Lock()
		count++
		mu.Unlock()
		wg.Done()
	}

	bus.Subscribe("event", handler)
	bus.Subscribe("event", handler)

	bus.Publish(core.Event{Type: "event"})
	wg.Wait()

	if count != 2 {
		t.Errorf("处理器调用次数 = %d, 期望 2", count)
	}
}

func TestBusDifferentTypes(t *testing.T) {
	bus := New()

	received := false
	bus.Subscribe("type.a", func(event core.Event) {
		received = true
	})

	bus.Publish(core.Event{Type: "type.b"})

	if received {
		t.Error("不应收到其他类型的事件")
	}
}

func TestBusUnsubscribeByID(t *testing.T) {
	bus := New()

	callCount := 0
	var mu sync.Mutex

	id := bus.Subscribe("test.unsub", func(event core.Event) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})

	// 第一次发布，应该被调用
	bus.Publish(core.Event{Type: "test.unsub"})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if callCount != 1 {
		t.Errorf("发布后期望调用 1 次，实际 %d 次", callCount)
	}
	mu.Unlock()

	// 取消订阅
	bus.Unsubscribe("test.unsub", id)

	// 第二次发布，不应被调用
	bus.Publish(core.Event{Type: "test.unsub"})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if callCount != 1 {
		t.Errorf("取消订阅后期望调用仍为 1 次，实际 %d 次", callCount)
	}
	mu.Unlock()
}

// TestBusConcurrentStress 验证高并发发布、订阅与取消订阅无数据竞争与死锁
func TestBusConcurrentStress(t *testing.T) {
	bus := New()
	const numEvents = 200
	const numPublishers = 5

	var receivedCount atomic.Int64

	// 注册 5 个订阅者
	var subIDs []core.SubscriptionID
	for i := 0; i < 5; i++ {
		id := bus.Subscribe("stress.event", func(ev core.Event) {
			receivedCount.Add(1)
		})
		subIDs = append(subIDs, id)
	}

	var wg sync.WaitGroup
	// 并发发布者
	for p := 0; p < numPublishers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numEvents; i++ {
				bus.Publish(core.Event{Type: "stress.event"})
			}
		}()
	}

	// 动态并发订阅与退订，验证切片读写竞态隔离
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			tmpID := bus.Subscribe("stress.event", func(ev core.Event) {})
			time.Sleep(time.Millisecond)
			bus.Unsubscribe("stress.event", tmpID)
		}
	}()

	wg.Wait()
	bus.Wait()

	if count := receivedCount.Load(); count < int64(numEvents*numPublishers) {
		t.Errorf("收到事件数 %d 低于预期", count)
	}
}

// TestBusPanicRecovery 验证订阅者触发 panic 时不破坏其他订阅者与总线正常运转
func TestBusPanicRecovery(t *testing.T) {
	bus := New()

	var normalDone atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// 恶意抛出 panic 的订阅者
	bus.Subscribe("panic.event", func(ev core.Event) {
		panic("simulated fatal handler panic")
	})

	// 正常订阅者
	bus.Subscribe("panic.event", func(ev core.Event) {
		normalDone.Store(true)
		wg.Done()
	})

	bus.Publish(core.Event{Type: "panic.event"})
	wg.Wait()

	if !normalDone.Load() {
		t.Error("正常订阅者应在并发或后置订阅者发生 panic 时依旧正常执行")
	}
}

// TestBusTimeoutAndClose 验证超时截断与优雅关闭
func TestBusTimeoutAndClose(t *testing.T) {
	bus := New()
	bus.SetHandlerTimeout(50 * time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(1)

	// 故意超时的阻塞订阅者
	bus.Subscribe("hang.event", func(ev core.Event) {
		defer wg.Done()
		time.Sleep(200 * time.Millisecond)
	})

	bus.Publish(core.Event{Type: "hang.event"})
	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := bus.Close(ctx); err != nil {
		t.Errorf("总线应正常关闭: %v", err)
	}
}
