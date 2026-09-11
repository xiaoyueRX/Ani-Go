package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// AnimeOpsDeps 核心业务工具所需的依赖
type AnimeOpsDeps struct {
	Downloader        core.Downloader
	MultiSource       core.Source
	TriggerSupplement func(ctx context.Context, subID uint) error
	TriggerRefresh    func()
}

// RegisterAnimeOpsTools 注册标准追番与下载业务工具
func (s *Server) RegisterAnimeOpsTools(deps AnimeOpsDeps) {
	// 1. search_anime
	s.RegisterTool(Tool{
		Name:        "search_anime",
		Description: "全网聚合搜索番剧资源，支持 Mikan / Bangumi 等源",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"keyword": {
					Type:        "string",
					Description: "搜索关键词，例如番剧中文名或日文名",
				},
				"source": {
					Type:        "string",
					Description: "数据源名称（可选，如 mikan, bangumi, animetosho）",
				},
			},
			Required: []string{"keyword"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		kw, _ := args["keyword"].(string)
		if strings.TrimSpace(kw) == "" {
			return ErrorResult("关键词不能为空"), nil
		}

		if deps.MultiSource == nil {
			return ErrorResult("聚合搜索源未配置"), nil
		}

		items, err := deps.MultiSource.SearchAnime(ctx, kw)
		if err != nil {
			return ErrorResult(fmt.Sprintf("搜索失败: %v", err)), nil
		}

		if len(items) == 0 {
			return TextResult(fmt.Sprintf("未找到关键词 %q 的相关番剧资源", kw)), nil
		}

		// 限制返回最多 10 条，避免 Token 爆炸
		limit := 10
		if len(items) < limit {
			limit = len(items)
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("共找到 %d 条关于 %q 的资源（展示前 %d 条）：\n\n", len(items), kw, limit))
		for i := 0; i < limit; i++ {
			item := items[i]
			sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, item.Title))
			if item.GroupName != "" {
				sb.WriteString(fmt.Sprintf("   - 字幕组: %s\n", item.GroupName))
			}
			if item.Size > 0 {
				sb.WriteString(fmt.Sprintf("   - 大小: %.2f MB\n", float64(item.Size)/(1024*1024)))
			}
			if item.URL != "" {
				sb.WriteString(fmt.Sprintf("   - 链接: %s\n", item.URL))
			} else if item.MagnetURL != "" {
				sb.WriteString(fmt.Sprintf("   - 磁力: %s\n", item.MagnetURL))
			}
		}

		return TextResult(sb.String()), nil
	})

	// 2. list_subscriptions
	s.RegisterTool(Tool{
		Name:        "list_subscriptions",
		Description: "查询当前所有追番订阅状态、更新季度与集数进度",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"enabled_only": {
					Type:        "boolean",
					Description: "是否仅列出已启用的订阅（默认为 false，列出全部）",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		if database.DB == nil {
			return ErrorResult("数据库未就绪"), nil
		}

		enabledOnly, _ := args["enabled_only"].(bool)
		var subs []database.Subscription

		query := database.DB.Model(&database.Subscription{})
		if enabledOnly {
			query = query.Where("enabled = ? OR enabled IS NULL", true)
		}

		if err := query.Order("id desc").Find(&subs).Error; err != nil {
			return ErrorResult(fmt.Sprintf("查询订阅失败: %v", err)), nil
		}

		if len(subs) == 0 {
			return TextResult("当前没有任何追番订阅"), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📋 当前订阅列表 (共 %d 部)：\n\n", len(subs)))
		for _, sub := range subs {
			status := "🟢 追番中"
			if sub.Completed {
				status = "🏁 已完结"
			} else if sub.Enabled != nil && !*sub.Enabled {
				status = "⏸️ 已暂停"
			}

			sb.WriteString(fmt.Sprintf("• [ID: %d] **%s** (第 %d 季) - %s\n", sub.ID, sub.TitleCN, sub.Season, status))
			sb.WriteString(fmt.Sprintf("  进度: %d/%d 集", sub.CurrentEpisodes, sub.TotalEpisodes))
			if sub.SubgroupName != "" {
				sb.WriteString(fmt.Sprintf(" | 字幕组: %s", sub.SubgroupName))
			}
			sb.WriteString("\n")
		}

		return TextResult(sb.String()), nil
	})

	// 3. add_subscription
	s.RegisterTool(Tool{
		Name:        "add_subscription",
		Description: "新增一个番剧追番订阅，支持自动匹配规则",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"title": {
					Type:        "string",
					Description: "番剧中文标题",
				},
				"season": {
					Type:        "integer",
					Description: "季号（默认为 1）",
				},
				"rss_url": {
					Type:        "string",
					Description: "Mikan 或其他站点的 RSS 订阅链接（可选）",
				},
				"subgroup": {
					Type:        "string",
					Description: "字幕组偏好名称（可选）",
				},
			},
			Required: []string{"title"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		if database.DB == nil {
			return ErrorResult("数据库未就绪"), nil
		}

		title, _ := args["title"].(string)
		title = strings.TrimSpace(title)
		if title == "" {
			return ErrorResult("番剧标题不能为空"), nil
		}

		season := 1
		if sVal, ok := args["season"].(float64); ok && int(sVal) > 0 {
			season = int(sVal)
		}
		rssURL, _ := args["rss_url"].(string)
		subgroup, _ := args["subgroup"].(string)

		enabled := true
		sub := database.Subscription{
			TitleCN:      title,
			Season:       season,
			RSSURL:       strings.TrimSpace(rssURL),
			SubgroupName: strings.TrimSpace(subgroup),
			Enabled:      &enabled,
		}

		if err := database.DB.Create(&sub).Error; err != nil {
			return ErrorResult(fmt.Sprintf("创建订阅失败: %v", err)), nil
		}

		return TextResult(fmt.Sprintf("✅ 成功创建追番订阅: [ID: %d] %s (第 %d 季)", sub.ID, sub.TitleCN, sub.Season)), nil
	})

	// 4. remove_subscription
	s.RegisterTool(Tool{
		Name:        "remove_subscription",
		Description: "取消订阅指定番剧",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"subscription_id": {
					Type:        "integer",
					Description: "要取消订阅的番剧 ID",
				},
			},
			Required: []string{"subscription_id"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		if database.DB == nil {
			return ErrorResult("数据库未就绪"), nil
		}

		idVal, ok := args["subscription_id"].(float64)
		if !ok || idVal <= 0 {
			return ErrorResult("请提供有效的 subscription_id"), nil
		}
		id := uint(idVal)

		var sub database.Subscription
		if err := database.DB.First(&sub, id).Error; err != nil {
			return ErrorResult(fmt.Sprintf("未找到 ID 为 %d 的订阅", id)), nil
		}

		if err := database.DB.Delete(&sub).Error; err != nil {
			return ErrorResult(fmt.Sprintf("删除订阅失败: %v", err)), nil
		}

		return TextResult(fmt.Sprintf("🗑️ 已成功取消订阅: %s (ID: %d)", sub.TitleCN, id)), nil
	})

	// 5. get_downloads
	s.RegisterTool(Tool{
		Name:        "get_downloads",
		Description: "实时查看下载器中的任务队列、下载速度与进度",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"status": {
					Type:        "string",
					Description: "筛选状态: all(全部), downloading(下载中), completed(已完成)",
					Enum:        []string{"all", "downloading", "completed"},
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		if deps.Downloader == nil {
			return ErrorResult("下载器未配置或未连接"), nil
		}

		tasks, err := deps.Downloader.List(ctx)
		if err != nil {
			return ErrorResult(fmt.Sprintf("获取下载列表失败: %v", err)), nil
		}

		statusFilter, _ := args["status"].(string)
		if statusFilter == "" {
			statusFilter = "all"
		}

		var filtered []core.DownloadTask
		for _, t := range tasks {
			if statusFilter == "downloading" && t.Progress >= 1.0 {
				continue
			}
			if statusFilter == "completed" && t.Progress < 1.0 {
				continue
			}
			filtered = append(filtered, t)
		}

		if len(filtered) == 0 {
			return TextResult(fmt.Sprintf("当前没有符合条件 (%s) 的下载任务", statusFilter)), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📥 当前任务列表 (共 %d 条)：\n\n", len(filtered)))
		for i, t := range filtered {
			speedStr := ""
			if t.SpeedDown > 0 {
				speedStr = fmt.Sprintf(" | 速度: %.1f KB/s", float64(t.SpeedDown)/1024)
			}
			sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, t.Name))
			sb.WriteString(fmt.Sprintf("   进度: %.1f%%%s | 状态: %s\n", t.Progress*100, speedStr, t.Status))
		}

		return TextResult(sb.String()), nil
	})

	// 6. trigger_refresh
	s.RegisterTool(Tool{
		Name:        "trigger_refresh",
		Description: "手动触发一次全系统 RSS 订阅源检查与自动下载调度",
		InputSchema: InputSchema{
			Type:       "object",
			Properties: map[string]Property{},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		if deps.TriggerRefresh != nil {
			go deps.TriggerRefresh()
			return TextResult("🚀 已触发全量 RSS 检查任务，后台正在更新中..."), nil
		}
		return TextResult("ℹ️ 系统当前处于轮询状态"), nil
	})
}
