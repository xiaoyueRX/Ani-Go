package event

import (
	"sync"
	"testing"

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
