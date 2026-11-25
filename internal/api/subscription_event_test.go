package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type recordingEventBus struct {
	mu     sync.Mutex
	events []core.Event
}

func (b *recordingEventBus) Publish(ev core.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func (b *recordingEventBus) Subscribe(string, core.EventHandler) core.SubscriptionID { return 1 }
func (b *recordingEventBus) Unsubscribe(string, core.SubscriptionID)               {}

func (b *recordingEventBus) snapshot() []core.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]core.Event, len(b.events))
	copy(out, b.events)
	return out
}

func TestSubscriptionHandlersPublishSubscriptionAddedEvent(t *testing.T) {
	setupTestDB(t)
	bus := &recordingEventBus{}
	s := &Server{eventBus: bus}

	// 1. 单个创建订阅
	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions", bytes.NewBufferString(`{"title_cn":"孤独摇滚！","bangumi_id":"328609"}`))
	w := httptest.NewRecorder()
	s.handleCreateSubscription(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("创建订阅失败: %d %s", w.Code, w.Body.String())
	}

	var created subscriptionResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	events := bus.snapshot()
	if len(events) != 1 {
		t.Fatalf("创建单个订阅应发布 1 个事件，实际 %d", len(events))
	}
	if events[0].Type != core.EventSubscriptionAdded {
		t.Fatalf("事件类型应为 %q，实际 %q", core.EventSubscriptionAdded, events[0].Type)
	}
	if id, _ := events[0].Payload["subscription_id"].(uint); id != created.ID || id == 0 {
		t.Fatalf("事件 subscription_id 异常: got=%v want=%d", events[0].Payload["subscription_id"], created.ID)
	}
	if title, _ := events[0].Payload["title"].(string); title != "孤独摇滚！" {
		t.Fatalf("事件 title 异常: %q", title)
	}

	// 2. 批量创建订阅
	batchReq := httptest.NewRequest(http.MethodPost, "/api/subscriptions/batch", bytes.NewBufferString(`{"items":[{"title_cn":"番剧甲","bangumi_id":"1001"},{"title_cn":"番剧乙","bangumi_id":"1002"}]}`))
	batchW := httptest.NewRecorder()
	s.handleBatchCreateSubscriptions(batchW, batchReq)
	if batchW.Code != http.StatusCreated {
		t.Fatalf("批量创建订阅失败: %d %s", batchW.Code, batchW.Body.String())
	}

	allEvents := bus.snapshot()
	if len(allEvents) != 3 {
		t.Fatalf("批量创建 2 个订阅后累计事件应为 3 个，实际 %d", len(allEvents))
	}
	for i := 1; i < len(allEvents); i++ {
		if allEvents[i].Type != core.EventSubscriptionAdded {
			t.Errorf("第 %d 个事件类型应为 %q，实际 %q", i, core.EventSubscriptionAdded, allEvents[i].Type)
		}
		if id, _ := allEvents[i].Payload["subscription_id"].(uint); id == 0 {
			t.Errorf("第 %d 个事件缺少 subscription_id", i)
		}
	}
}
