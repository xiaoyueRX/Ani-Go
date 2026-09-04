package v2

import (
	"strings"
	"testing"
)

func TestOrganizedNotification_SeparatesSuccessAndFailure(t *testing.T) {
	msg := organizedNotification(map[string]interface{}{
		"success":    2,
		"failed":     1,
		"final_path": "/media/anime/a.mkv",
	})

	if msg.Title != "📁 文件整理结果" {
		t.Fatalf("标题 = %q", msg.Title)
	}
	if !strings.Contains(msg.Content, "成功: 2 个") || !strings.Contains(msg.Content, "失败: 1 个") {
		t.Fatalf("内容 = %q", msg.Content)
	}
}

func TestOrganizedNotification_AllFailuresUseAlert(t *testing.T) {
	msg := organizedNotification(map[string]interface{}{"success": 0, "failed": 3})

	if msg.Title != "🚨 文件整理失败" || strings.Contains(msg.Content, "成功提示") {
		t.Fatalf("通知 = %+v", msg)
	}
	if msg.Priority != 2 {
		t.Fatalf("优先级 = %d, 期望 2", msg.Priority)
	}
}

func TestOrganizedNotification_ZeroCountsSuppressed(t *testing.T) {
	msg := organizedNotification(map[string]interface{}{"success": 0, "failed": 0})
	if msg != nil {
		t.Fatalf("当 success=0 && failed=0 时应静默返回 nil，实际得到: %+v", msg)
	}
}
