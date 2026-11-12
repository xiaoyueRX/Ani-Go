package mcp

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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

		pluginID, hasID, err := parseStringArg(args, "plugin_id")
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		pluginID = strings.TrimSpace(pluginID)
		rawEnabled, hasEnabled := args["enabled"]
		enabled, ok := rawEnabled.(bool)
		if !hasID || pluginID == "" || !hasEnabled || !ok {
			return ErrorResult("参数不合法: 需要提供有效的 plugin_id 和布尔型 enabled"), nil
		}

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
			dlCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if _, err := deps.Downloader.List(dlCtx); err == nil {
				dlStatus = "🟢 连接正常"
			} else {
				dlStatus = fmt.Sprintf("🔴 异常 (%v)", err)
			}
			cancel()
		}

		ver := strings.TrimPrefix(strings.TrimSpace(deps.Version), "v")
		if ver == "" {
			ver = defaultServerVersion
		}

		subSummary := "数据库未就绪"
		if database.DB != nil {
			var subCount int64
			if err := database.DB.WithContext(ctx).Model(&database.Subscription{}).Count(&subCount).Error; err != nil {
				subSummary = fmt.Sprintf("查询失败 (%v)", err)
			} else {
				subSummary = fmt.Sprintf("%d 部", subCount)
			}
		}

		var sb strings.Builder
		sb.WriteString("🖥️ **Ani-Go 系统健康报告**\n\n")
		sb.WriteString(fmt.Sprintf("- **内核版本**: v%s (纯 Go 单二进制)\n", ver))
		sb.WriteString(fmt.Sprintf("- **当前内存占用 (Alloc)**: %.2f MB\n", float64(m.Alloc)/(1024*1024)))
		sb.WriteString(fmt.Sprintf("- **系统堆内存 (Sys)**: %.2f MB\n", float64(m.Sys)/(1024*1024)))
		sb.WriteString(fmt.Sprintf("- **活跃 Goroutines**: %d\n", runtime.NumGoroutine()))
		sb.WriteString(fmt.Sprintf("- **下载器状态**: %s\n", dlStatus))
		sb.WriteString(fmt.Sprintf("- **当前总订阅数**: %s\n", subSummary))

		return TextResult(sb.String()), nil
	})

	// 4. update_system_config
	s.RegisterTool(Tool{
		Name:        "update_system_config",
		Description: "更新非敏感系统配置参数（保存至 settings 键值表）",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key": {
					Type:        "string",
					Description: "配置项名称，如 TV_BASE_PATH, DOWNLOADER_TYPE 等（禁止修改密钥与密码）",
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

		key, hasKey, err := parseStringArg(args, "key")
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		key = strings.TrimSpace(key)
		if !hasKey || key == "" {
			return ErrorResult("配置项 key 不能为空"), nil
		}
		if utf8.RuneCountInString(key) > 128 {
			return ErrorResult("配置项 key 长度不能超过 128 个字符"), nil
		}
		if hasInvalidKeyChars(key) {
			return ErrorResult("配置项 key 包含非法空白或控制字符"), nil
		}

		// 安全隔离：禁止通过 MCP 读取或篡改内部鉴权密钥、凭证、密码与插件运行态
		if isProtectedConfigKey(key) {
			return ErrorResult(fmt.Sprintf("安全限制：禁止通过 MCP 修改受保护或敏感配置项 %q", key)), nil
		}

		val, hasVal, err := parseStringArg(args, "value")
		if err != nil || !hasVal {
			return ErrorResult("配置项 value 必须是字符串"), nil
		}
		if utf8.RuneCountInString(val) > 4096 {
			return ErrorResult("配置项 value 长度不能超过 4096 个字符"), nil
		}

		setting := database.Setting{Key: key, Value: val}
		if err := database.DB.WithContext(ctx).Save(&setting).Error; err != nil {
			return ErrorResult(fmt.Sprintf("保存配置失败: %v", err)), nil
		}

		return TextResult(fmt.Sprintf("✅ 已更新配置项: %s = %q", key, val)), nil
	})
}

func hasInvalidKeyChars(key string) bool {
	for _, r := range key {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// isProtectedConfigKey 判断配置键是否属于敏感凭证或受保护的内部系统状态
func isProtectedConfigKey(key string) bool {
	canon := strings.ToUpper(strings.TrimSpace(key))
	canon = strings.ReplaceAll(canon, "-", "_")
	if canon == "" {
		return true
	}

	switch canon {
	case "JWT_SECRET",
		"TOKEN_SECRET",
		"MCP_TOKEN",
		"SESSION_SECRET",
		"AUTH_SECRET",
		"PLUGIN_STATUS",
		"CUSTOM_PLUGINS",
		"PASSWORD_HASH",
		"SERVERCHAN_KEY",
		"BARK_KEY",
		"BARK_DEVICE_KEY",
		"PUSHDEER_KEY":
		return true
	}

	sensitiveFragments := []string{
		"SECRET",
		"PASS",
		"PWD",
		"TOKEN",
		"API_KEY",
		"APIKEY",
		"PRIVATE_KEY",
		"ACCESS_KEY",
		"DEVICE_KEY",
		"SIGNING_KEY",
		"ENCRYPTION_KEY",
		"AUTH_KEY",
		"CREDENTIAL",
	}
	for _, frag := range sensitiveFragments {
		if strings.Contains(canon, frag) {
			return true
		}
	}

	return false
}
