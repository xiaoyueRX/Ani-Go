package mcp

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

// GovernanceDeps 系统治理工具所需的依赖
type GovernanceDeps struct {
	PluginManager *plugin.Manager
	Downloader    core.Downloader
	Version       string
}

// RegisterGovernanceTools 注册系统与插件治理工具
func (s *Server) RegisterGovernanceTools(deps GovernanceDeps) {
	// 1. list_plugins
	s.RegisterTool(Tool{
		Name:        "list_plugins",
		Description: "查询系统中所有已安装插件的运行状态与基本信息",
		InputSchema: InputSchema{
			Type:       "object",
			Properties: map[string]Property{},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		if deps.PluginManager == nil {
			return ErrorResult("插件管理器未启用"), nil
		}

		plugins := deps.PluginManager.List()
		if len(plugins) == 0 {
			return TextResult("当前系统没有安装任何插件"), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("🧩 插件列表 (共 %d 个)：\n\n", len(plugins)))
		for _, p := range plugins {
			status := "🟢 已启用"
			if !p.Enabled {
				status = "⚪ 已停用 (零协程/零资源消耗)"
			}
			sb.WriteString(fmt.Sprintf("• **%s** (%s) - %s\n", p.Name, p.ID, status))
			if p.Description != "" {
				sb.WriteString(fmt.Sprintf("  描述: %s\n", p.Description))
			}
			if p.Version != "" {
				sb.WriteString(fmt.Sprintf("  版本: %s\n", p.Version))
			}
		}

		return TextResult(sb.String()), nil
	})

	// 2. toggle_plugin
	s.RegisterTool(Tool{
		Name:        "toggle_plugin",
		Description: "远程启用或停用指定插件（及时释放后台资源）",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"plugin_id": {
					Type:        "string",
					Description: "插件 ID (例如 extended-notifiers)",
				},
				"enabled": {
					Type:        "boolean",
					Description: "目标状态: true 为启用, false 为停用",
				},
			},
			Required: []string{"plugin_id", "enabled"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		if deps.PluginManager == nil {
			return ErrorResult("插件管理器未启用"), nil
		}

		pluginID, _ := args["plugin_id"].(string)
		enabled, ok := args["enabled"].(bool)
		if strings.TrimSpace(pluginID) == "" || !ok {
			return ErrorResult("参数不合法: 需要提供 plugin_id 和 enabled"), nil
		}

		var err error
		if enabled {
			err = deps.PluginManager.Enable(pluginID)
		} else {
			err = deps.PluginManager.Disable(pluginID)
		}

		if err != nil {
			return ErrorResult(fmt.Sprintf("切换插件状态失败: %v", err)), nil
		}

		action := "启用"
		if !enabled {
			action = "停用"
		}
		return TextResult(fmt.Sprintf("✅ 已成功%s插件: %s", action, pluginID)), nil
	})

	// 3. get_system_status
	s.RegisterTool(Tool{
		Name:        "get_system_status",
		Description: "查询 Ani-Go 系统健康指标、内存占用、下载器连通性与版本",
		InputSchema: InputSchema{
			Type:       "object",
			Properties: map[string]Property{},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		dlStatus := "未配置"
		if deps.Downloader != nil {
			if _, err := deps.Downloader.List(ctx); err == nil {
				dlStatus = "🟢 连接正常"
			} else {
				dlStatus = fmt.Sprintf("🔴 异常 (%v)", err)
			}
		}

		ver := deps.Version
		if ver == "" {
			ver = "0.5.4"
		}

		var subCount int64
		if database.DB != nil {
			database.DB.Model(&database.Subscription{}).Count(&subCount)
		}

		var sb strings.Builder
		sb.WriteString("🖥️ **Ani-Go 系统健康报告**\n\n")
		sb.WriteString(fmt.Sprintf("- **内核版本**: v%s (纯 Go 单二进制)\n", ver))
		sb.WriteString(fmt.Sprintf("- **当前内存占用 (Alloc)**: %.2f MB\n", float64(m.Alloc)/(1024*1024)))
		sb.WriteString(fmt.Sprintf("- **系统堆内存 (Sys)**: %.2f MB\n", float64(m.Sys)/(1024*1024)))
		sb.WriteString(fmt.Sprintf("- **活跃 Goroutines**: %d\n", runtime.NumGoroutine()))
		sb.WriteString(fmt.Sprintf("- **下载器状态**: %s\n", dlStatus))
		sb.WriteString(fmt.Sprintf("- **当前总订阅数**: %d 部\n", subCount))

		return TextResult(sb.String()), nil
	})

	// 4. update_system_config
	s.RegisterTool(Tool{
		Name:        "update_system_config",
		Description: "更新系统配置参数（保存至 settings 键值表）",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key": {
					Type:        "string",
					Description: "配置项名称，如 TV_BASE_PATH, DOWNLOADER_TYPE 等",
				},
				"value": {
					Type:        "string",
					Description: "配置项的最新取值",
				},
			},
			Required: []string{"key", "value"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		if database.DB == nil {
			return ErrorResult("数据库未就绪"), nil
		}

		key, _ := args["key"].(string)
		val, _ := args["value"].(string)
		key = strings.TrimSpace(key)
		if key == "" {
			return ErrorResult("配置项 key 不能为空"), nil
		}

		setting := database.Setting{Key: key, Value: val}
		if err := database.DB.Save(&setting).Error; err != nil {
			return ErrorResult(fmt.Sprintf("保存配置失败: %v", err)), nil
		}

		return TextResult(fmt.Sprintf("✅ 已更新配置项: %s = %q", key, val)), nil
	})
}
