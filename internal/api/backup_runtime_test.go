package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/backup"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

func TestConcurrentBackupsReloadAndEventDispatch(t *testing.T) {
	setupTestDB(t)
	b := event.New()
	m := plugin.NewManager(b)
	t.Cleanup(m.StopAll)
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	cfg := new(config.Config)
	cfg.Scheduler.BackupPath = t.TempDir()
	cfg.Scheduler.BackupKeepCount = 50
	bm := backup.NewBackupManager(cfg)
	if err := bm.CreateBackup(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	fs, err := bm.ListBackups()
	if err != nil || len(fs) != 1 {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"name": fs[0].Name})
	payload := string(body)
	mux := http.NewServeMux()
	(&Server{pluginManager: m, backupManager: bm}).registerRoutes(mux)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, work := range []func(){
		func() {
			for i := 0; i < 24; i++ {
				b.Publish(core.Event{Type: "data.migrate", Payload: map[string]any{"subscriptions": 1}})
				_ = m.GetPluginList()
			}
		},
		func() {
			for i := 0; i < 8; i++ {
				w := httptest.NewRecorder()
				r := httptest.NewRequest("POST", "/api/backup/restore", strings.NewReader(payload))
				mux.ServeHTTP(w, r.WithContext(ctx))
				if w.Code != 200 {
					t.Errorf("恢复返回 %d", w.Code)
					return
				}
			}
		},
		func() {
			for i := 0; i < 8; i++ {
				if err := m.Reload(); err != nil {
					t.Error(err)
					return
				}
				if err := bm.CreateBackup(ctx, true); err != nil {
					t.Error(err)
					return
				}
			}
		},
	} {
		wg.Add(1)
		go func() { defer wg.Done(); work() }()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("备份、重载与订阅事件未在期限内完成")
	}
	if err := b.Close(ctx); err != nil {
		t.Fatal(err)
	}
	files, err := bm.ListBackups()
	if err != nil || len(files) != 9 || !m.IsPluginActive("data-migrate") {
		t.Fatalf("并发后的备份数或迁移插件有误，备份数 %d，错误 %v", len(files), err)
	}
}
