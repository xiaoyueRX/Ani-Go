package api

import (
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"testing"
)

func TestCommittedSettingsWakeSchedulerOnlyForIntervals(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()
	calls := 0
	s.reloadIntervals = func() {
		calls++
		var setting database.Setting
		if err := database.DB.First(&setting, "key = ?", "RSS_INTERVAL").Error; err != nil || setting.Value != "2h" {
			t.Errorf("回调早于设置提交: %v", err)
		}
	}
	for _, req := range []string{`{"settings":{"RSS_INTERVAL":"0s"}}`, `{"settings":{"TZ":"UTC"}}`, `{"settings":{"RSS_INTERVAL":"2h"}}`} {
		webRequest(s, "PUT", "/api/settings", req)
	}
	if calls != 1 {
		t.Fatalf("设置唤醒次数 %d", calls)
	}
}
