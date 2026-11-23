package plugin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
)

func TestWebhookBurstDrainsThroughSubscriptionAndCancelsOnDisable(t *testing.T) {
	setupTestDB(t)
	b := event.New()
	manager := NewManager(b)
	manager.allowPrivateWebhookForTest = true
	t.Cleanup(manager.StopAll)
	entered := make(chan struct{}, 12)
	finalized := make(chan struct{}, 12)
	release := make(chan struct{})
	var active atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		active.Add(1)
		entered <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}
		active.Add(-1)
		finalized <- struct{}{}
	}))
	defer srv.Close()
	defer close(release)
	if err := manager.AddOrUpdatePlugin(PluginInfo{ID: "local-hook", Name: "本地订阅", Type: "webhook", URL: srv.URL, Events: []string{"organized"}}); err != nil {
		t.Fatal(err)
	}
	for range 12 {
		b.Publish(core.Event{Type: "organized"})
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("本地服务没有收到投递")
	}
	select {
	case <-entered:
		t.Errorf("一个阻塞的 Webhook 占用并发连接")
	case <-time.After(40 * time.Millisecond):
	}
	if err := manager.Disable("local-hook"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finalized:
	case <-time.After(time.Second):
		t.Fatal("停用未取消在途请求")
	}
	drain, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := b.Close(drain); err != nil {
		t.Fatal(err)
	}
}
