package event

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestCloseWaitsForActualCallbackAfterTimeout(t *testing.T) {
	b := New()
	b.SetHandlerTimeout(time.Millisecond)
	started, release := make(chan struct{}), make(chan struct{})
	b.Subscribe("organized", func(core.Event) { close(started); <-release })
	b.Publish(core.Event{Type: "organized"})
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Millisecond)
	err := b.Close(ctx)
	cancel()
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("执行中的回调尚未退出，Close 返回 %v", err)
	}
	if err := b.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestBlockedSubscriberCannotFanOutItsCallbacks(t *testing.T) {
	b := New()
	var running atomic.Int32
	var overlapped atomic.Bool
	entered := make(chan struct{}, 20)
	release := make(chan struct{})
	b.Subscribe("episode", func(core.Event) {
		if running.Add(1) > 1 {
			overlapped.Store(true)
		}
		entered <- struct{}{}
		<-release
		running.Add(-1)
	})
	got := make(chan struct{}, 20)
	b.Subscribe("episode", func(core.Event) { got <- struct{}{} })
	for i := 0; i < 20; i++ {
		b.Publish(core.Event{Type: "episode"})
	}
	for i := 0; i < 20; i++ {
		select {
		case <-got:
		case <-time.After(time.Second):
			t.Fatal("慢订阅阻塞健康订阅")
		}
	}
	<-entered
	close(release)
	b.Wait()
	if overlapped.Load() {
		t.Fatal("同一订阅并行执行了回调")
	}
}

func TestSaturatedSubscriberReportsOnlyUnacceptedDeliveries(t *testing.T) {
	b := New()
	release := make(chan struct{})
	started := make(chan struct{})
	slow := make(chan int, queueCapacity+1)
	fast := make(chan int, 1)
	b.Subscribe("feed", func(e core.Event) {
		n := e.Payload["n"].(int)
		if n == 0 {
			close(started)
			<-release
		}
		slow <- n
	})
	b.Subscribe("feed", func(e core.Event) { fast <- e.Payload["n"].(int) })
	full := 0
	for n := 0; n < queueCapacity+8; n++ {
		result := b.TryPublish(core.Event{Type: "feed", Payload: map[string]any{"n": n}})
		want := 2
		if n > queueCapacity {
			want = 1
		}
		if result.Accepted != want || result.Full != 2-want || result.Closed {
			t.Fatalf("发布 %d 获得 %+v", n, result)
		}
		full += result.Full
		if got := <-fast; got != n {
			t.Fatal("健康订阅顺序错误", got, n)
		}
		if n == 0 {
			<-started
		}
	}
	if full != 7 {
		t.Fatal("队满统计不准确", full)
	}
	close(release)
	if err := b.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	for n := 0; n <= queueCapacity; n++ {
		if got := <-slow; got != n {
			t.Fatalf("已接纳记录 %d 被覆盖为 %d", n, got)
		}
	}
	if got := b.TryPublish(core.Event{Type: "feed"}); !got.Closed || got.Accepted != 0 {
		t.Fatal(got)
	}
	if id := b.Subscribe("after", func(core.Event) {}); id != 0 {
		t.Fatal("已关闭的总线接受新订阅")
	}
}

func TestSubscribeCallbackCanPublishRecursively(t *testing.T) {
	b := New()
	done := make(chan struct{})
	b.Subscribe("step", func(e core.Event) {
		if n := e.Payload["n"].(int); n == 32 {
			close(done)
		} else {
			b.Publish(core.Event{Type: "step", Payload: map[string]any{"n": n + 1}})
		}
	})
	b.Publish(core.Event{Type: "step", Payload: map[string]any{"n": 0}})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("递归发布自锁")
	}
	if err := b.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}
