package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	v2 "github.com/xiaoyueRX/Ani-Go/internal/notifier/v2"
)

type failingTestNotification struct{ count atomic.Int32 }

func (n *failingTestNotification) Name() string { return "Discord" }
func (n *failingTestNotification) Send(context.Context, string, string) error {
	n.count.Add(1)
	return errors.New("测试渠道投递失败")
}

func TestNotificationTestReportsOneAttemptAndMissingManager(t *testing.T) {
	setupTestDB(t)
	var fallbackAttempts atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackAttempts.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer remote.Close()
	if err := database.DB.Save(&database.Setting{Key: "DISCORD_WEBHOOK", Value: remote.URL}).Error; err != nil {
		t.Fatal(err)
	}
	notifier := new(failingTestNotification)
	manager := v2.NewNotifyManager([]v2.Notifier{notifier}, nil)
	manager.Start()
	defer manager.Stop()
	check := func(server *Server, payload string, wantSuccess bool) {
		t.Helper()
		w := httptest.NewRecorder()
		server.handleTestNotify(w, httptest.NewRequest(http.MethodPost, "/api/notify/test", strings.NewReader(payload)))
		var response struct {
			Success bool
			Error   string
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Success != wantSuccess || (!wantSuccess && response.Error == "") {
			t.Error("测试投递响应与实际结果不一致")
		}
	}
	check(&Server{notifyMgr: manager}, `{"channel":"discord"}`, false)
	if notifier.count.Load() != 1 || fallbackAttempts.Load() != 0 {
		t.Errorf("测试投递重复：配置渠道 %d 次，兼容投递 %d 次", notifier.count.Load(), fallbackAttempts.Load())
	}
	manager.ReloadNotifiers(nil)
	check(&Server{notifyMgr: manager}, `{"channel":"discord"}`, true)
	if fallbackAttempts.Load() != 1 {
		t.Fatal("未加载的兼容渠道应发送一次")
	}
	check(&Server{}, `{}`, false)
}
