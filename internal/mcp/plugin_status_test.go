package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

func TestGovernanceReportsPluginLifecycle(t *testing.T) {
	previous := database.DB
	database.DB = nil
	t.Cleanup(func() { database.DB = previous })
	manager := plugin.NewManager(nil)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.StopAll)
	server := NewServer(func() string { return "token" })
	server.RegisterGovernanceTools(GovernanceDeps{PluginManager: manager})

	call := func(tool string, args map[string]any) *ToolCallResult {
		t.Helper()
		result, err := server.CallTool(context.Background(), tool, args)
		if err != nil || result == nil || len(result.Content) == 0 {
			t.Fatalf("调用 %s 失败：%+v，%v", tool, result, err)
		}
		return result
	}
	toggle := func(enabled bool) string {
		t.Helper()
		result := call("toggle_plugin", map[string]any{"plugin_id": "data-migrate", "enabled": enabled})
		if result.IsError {
			t.Fatalf("切换失败：%s", result.Content[0].Text)
		}
		return result.Content[0].Text
	}
	listStatus := func() string {
		t.Helper()
		result := call("list_plugins", nil)
		for _, line := range strings.Split(result.Content[0].Text, "\n") {
			if strings.Contains(line, "`data-migrate`") {
				return line
			}
		}
		t.Fatal("插件列表缺少 data-migrate")
		return ""
	}

	for _, text := range []string{toggle(true), listStatus()} {
		for _, want := range []string{"已挂起", "suspended", string(core.ServiceEventBus)} {
			if !strings.Contains(text, want) {
				t.Fatalf("等待服务时缺少 %q：%s", want, text)
			}
		}
	}
	manager.SetCoreService(core.ServiceEventBus, &mockBus{})
	for _, text := range []string{listStatus(), toggle(true)} {
		if !strings.Contains(text, "active") || !strings.Contains(text, "运行中") {
			t.Fatalf("恢复后未显示运行状态：%s", text)
		}
	}
	for _, text := range []string{toggle(false), listStatus()} {
		if !strings.Contains(text, "inactive") || !strings.Contains(text, "已停用") {
			t.Fatalf("停用后状态有误：%s", text)
		}
	}
	if result := call("toggle_plugin", map[string]any{"plugin_id": "unknown-plugin", "enabled": true}); !result.IsError {
		t.Errorf("不存在的插件收到了成功回执：%s", result.Content[0].Text)
	}
}
