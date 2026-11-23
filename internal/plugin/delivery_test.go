package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestTogglePreservesNotificationContext(t *testing.T) {
	setupTestDB(t)
	mgr := NewManager(newTestBus())
	mgr.Load()
	defer mgr.StopAll()
	initial := mgr.GetPluginContext("extended-notifiers")
	if initial == nil {
		t.Fatal("未启动通知插件")
	}
	if err := mgr.Enable("yuc_schedule"); err != nil {
		t.Fatal(err)
	}
	if current := mgr.GetPluginContext("extended-notifiers"); current != initial || initial.Context().Err() != nil {
		t.Fatal("变更无关插件时重启了通知插件")
	}
}

func TestLegacyWebhookEventNames(t *testing.T) {
	setupTestDB(t)
	bus := newTestBus()
	mgr := NewManager(bus)
	mgr.allowPrivateWebhookForTest = true
	got := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg struct{ Event string }
		_ = json.NewDecoder(r.Body).Decode(&msg)
		got <- msg.Event
	}))
	defer server.Close()
	defer mgr.StopAll()
	if err := mgr.AddOrUpdatePlugin(PluginInfo{ID: "legacy", Name: "旧版", Type: "webhook", URL: server.URL, Events: []string{"episode.missing", "error"}}); err != nil {
		t.Fatal(err)
	}
	bus.Publish(core.Event{Type: core.EventSupplementCompleted, Payload: map[string]any{"new_count": 0}})
	bus.Publish(core.Event{Type: core.EventSupplementCompleted, Payload: map[string]any{"new_count": 1}})
	bus.Publish(core.Event{Type: core.EventDownloadFailed})
	seen := map[string]int{}
	for range 2 {
		select {
		case name := <-got:
			seen[name]++
		case <-time.After(time.Second):
			t.Fatalf("旧版订阅未收到事件: %v", seen)
		}
	}
	if seen["episode.missing"] != 1 || seen["error"] != 1 {
		t.Fatalf("通过别名投递的事件不正确: %v", seen)
	}
	select {
	case name := <-got:
		t.Errorf("重复投递: %s", name)
	case <-time.After(50 * time.Millisecond):
	}
}
