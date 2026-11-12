package mcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
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
					Description: "数据源名称（可选，如 mikan, bangumi, nyaa, acgrip, animetosho）",
				},
				"limit": {
					Type:        "integer",
					Description: "最大返回条数（默认 10，最大 50）",
				},
			},
			Required: []string{"keyword"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		kw, hasKW, err := parseStringArg(args, "keyword")
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		kw = strings.TrimSpace(kw)
		if !hasKW || kw == "" {
			return ErrorResult("关键词不能为空"), nil
		}
		if utf8.RuneCountInString(kw) > 200 {
			return ErrorResult("关键词长度不能超过 200 个字符"), nil
		}

		sourceFilter, _, err := parseStringArg(args, "source")
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		sourceFilter = strings.TrimSpace(sourceFilter)

		limit, _, err := parseBoundedIntArg(args, "limit", 10, 1, 50, true)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}

		if deps.MultiSource == nil {
			return ErrorResult("聚合搜索源未配置"), nil
		}

		items, err := deps.MultiSource.SearchAnime(ctx, kw)
		if err != nil {
			return ErrorResult(fmt.Sprintf("搜索失败: %v", err)), nil
		}

		if sourceFilter != "" {
			items = filterItemsBySource(items, sourceFilter)
		}

		if len(items) == 0 {
			if sourceFilter != "" {
				return TextResult(fmt.Sprintf("未在数据源 %q 中找到关键词 %q 的相关番剧资源", sourceFilter, kw)), nil
			}
			return TextResult(fmt.Sprintf("未找到关键词 %q 的相关番剧资源", kw)), nil
		}

		displayCount := limit
		if len(items) < displayCount {
			displayCount = len(items)
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("共找到 %d 条关于 %q 的资源（展示前 %d 条）：\n\n", len(items), kw, displayCount))
		for i := 0; i < displayCount; i++ {
			item := items[i]
			sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, item.Title))
			if item.SourceName != "" {
				sb.WriteString(fmt.Sprintf("   - 来源: %s\n", item.SourceName))
			}
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
				"limit": {
					Type:        "integer",
					Description: "每页返回数量（默认 50，最大 100）",
				},
				"offset": {
					Type:        "integer",
					Description: "分页偏移量（默认 0）",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		if database.DB == nil {
			return ErrorResult("数据库未就绪"), nil
		}

		enabledOnly, err := parseBoolArg(args, "enabled_only", false)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		limit, _, err := parseBoundedIntArg(args, "limit", 50, 1, 100, true)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		offset, _, err := parseBoundedIntArg(args, "offset", 0, 0, math.MaxInt32, false)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}

		query := database.DB.WithContext(ctx).Model(&database.Subscription{})
		if enabledOnly {
			query = query.Where("enabled = ? OR enabled IS NULL", true)
		}

		var total int64
		if err := query.Count(&total).Error; err != nil {
			return ErrorResult(fmt.Sprintf("查询订阅总数失败: %v", err)), nil
		}
		if total == 0 {
			return TextResult("当前没有任何追番订阅"), nil
		}

		var subs []database.Subscription
		if err := query.Order("id desc").Limit(limit).Offset(offset).Find(&subs).Error; err != nil {
			return ErrorResult(fmt.Sprintf("查询订阅失败: %v", err)), nil
		}
		if len(subs) == 0 {
			return TextResult(fmt.Sprintf("当前共有 %d 部订阅，偏移量 %d 超出范围", total, offset)), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📋 当前订阅列表 (共 %d 部，本次展示 %d-%d)：\n\n", total, offset+1, offset+len(subs)))
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
					Description: "季号（默认为 1，范围 1-100）",
				},
				"rss_url": {
					Type:        "string",
					Description: "Mikan 或其他站点的 RSS 订阅链接（可选，需为 http/https 链接）",
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

		title, hasTitle, err := parseStringArg(args, "title")
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		title = strings.TrimSpace(title)
		if !hasTitle || title == "" {
			return ErrorResult("番剧标题不能为空"), nil
		}
		if utf8.RuneCountInString(title) > 255 {
			return ErrorResult("番剧标题长度不能超过 255 个字符"), nil
		}

		season, _, err := parseBoundedIntArg(args, "season", 1, 1, 100, false)
		if err != nil {
			return ErrorResult("季号必须是 1~100 之间的整数"), nil
		}

		rssURL, _, err := parseStringArg(args, "rss_url")
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		rssURL = strings.TrimSpace(rssURL)
		if rssURL != "" {
			if len(rssURL) > 2048 {
				return ErrorResult("RSS 链接长度不能超过 2048 个字符"), nil
			}
			u, parseErr := url.ParseRequestURI(rssURL)
			if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return ErrorResult("rss_url 格式无效，仅支持 http/https 协议的完整链接"), nil
			}
		}

		subgroup, _, err := parseStringArg(args, "subgroup")
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		subgroup = strings.TrimSpace(subgroup)
		if utf8.RuneCountInString(subgroup) > 128 {
			return ErrorResult("字幕组名称不能超过 128 个字符"), nil
		}

		enabled := true
		sub := database.Subscription{
			TitleCN:      title,
			Season:       season,
			RSSURL:       rssURL,
			SubgroupName: subgroup,
			Enabled:      &enabled,
		}

		if err := database.DB.WithContext(ctx).Create(&sub).Error; err != nil {
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

		id, err := parseRequiredPositiveUint(args, "subscription_id")
		if err != nil {
			return ErrorResult("请提供有效的正整数 subscription_id"), nil
		}

		var sub database.Subscription
		if err := database.DB.WithContext(ctx).First(&sub, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrorResult(fmt.Sprintf("未找到 ID 为 %d 的订阅", id)), nil
			}
			return ErrorResult(fmt.Sprintf("查询订阅失败: %v", err)), nil
		}

		// 手动在事务中级联删除关联剧集记录，避免软删除后留下孤立剧集数据
		if err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Delete(&sub).Error; err != nil {
				return err
			}
			if tx.Migrator().HasTable(&database.Episode{}) {
				if err := tx.Where("subscription_id = ?", id).Delete(&database.Episode{}).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
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
				"limit": {
					Type:        "integer",
					Description: "最大返回条数（默认 20，最大 100）",
				},
				"offset": {
					Type:        "integer",
					Description: "分页偏移量（默认 0）",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		if deps.Downloader == nil {
			return ErrorResult("下载器未配置或未连接"), nil
		}

		statusFilter, _, err := parseStringArg(args, "status")
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		statusFilter = strings.TrimSpace(statusFilter)
		if statusFilter == "" {
			statusFilter = "all"
		}
		switch statusFilter {
		case "all", "downloading", "completed":
		default:
			return ErrorResult(fmt.Sprintf("不支持的下载状态筛选值 %q，仅支持 all, downloading, completed", statusFilter)), nil
		}

		limit, _, err := parseBoundedIntArg(args, "limit", 20, 1, 100, true)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		offset, _, err := parseBoundedIntArg(args, "offset", 0, 0, math.MaxInt32, false)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}

		tasks, err := deps.Downloader.List(ctx)
		if err != nil {
			return ErrorResult(fmt.Sprintf("获取下载列表失败: %v", err)), nil
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

		if offset >= len(filtered) {
			return TextResult(fmt.Sprintf("当前共有 %d 条符合条件 (%s) 的下载任务，偏移量 %d 超出范围", len(filtered), statusFilter, offset)), nil
		}

		end := offset + limit
		if end > len(filtered) {
			end = len(filtered)
		}
		page := filtered[offset:end]

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📥 当前任务列表 (共 %d 条，本次展示 %d-%d)：\n\n", len(filtered), offset+1, end))
		for i, t := range page {
			speedStr := ""
			if t.SpeedDown > 0 {
				speedStr = fmt.Sprintf(" | 速度: %.1f KB/s", float64(t.SpeedDown)/1024)
			}
			sb.WriteString(fmt.Sprintf("%d. **%s**\n", offset+i+1, t.Name))
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
		if deps.TriggerRefresh == nil {
			return ErrorResult("刷新触发器未配置"), nil
		}
		// 后台异步触发全量刷新，并挂载 recover 防止回调 panic 影响主进程
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("❌ [MCP] trigger_refresh 后台执行 panic: %v", r)
				}
			}()
			deps.TriggerRefresh()
		}()
		return TextResult("🚀 已触发全量 RSS 检查任务，后台正在更新中..."), nil
	})
}

func filterItemsBySource(items []core.TorrentItem, sourceFilter string) []core.TorrentItem {
	normFilter := normalizeSourceName(sourceFilter)
	if normFilter == "" {
		return items
	}
	filtered := make([]core.TorrentItem, 0, len(items))
	for _, item := range items {
		normSource := normalizeSourceName(item.SourceName)
		if normSource != "" && (normSource == normFilter || strings.Contains(normSource, normFilter)) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func normalizeSourceName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func parseStringArg(args map[string]interface{}, key string) (string, bool, error) {
	if args == nil {
		return "", false, nil
	}
	raw, exists := args[key]
	if !exists || raw == nil {
		return "", false, nil
	}
	val, ok := raw.(string)
	if !ok {
		return "", true, fmt.Errorf("参数 %s 必须是字符串", key)
	}
	return val, true, nil
}

func parseBoolArg(args map[string]interface{}, key string, defaultVal bool) (bool, error) {
	if args == nil {
		return defaultVal, nil
	}
	raw, exists := args[key]
	if !exists || raw == nil {
		return defaultVal, nil
	}
	val, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("参数 %s 必须是布尔值", key)
	}
	return val, nil
}

func parseBoundedIntArg(args map[string]interface{}, key string, defaultVal, minVal, maxVal int, clampMax bool) (int, bool, error) {
	if args == nil {
		return defaultVal, false, nil
	}
	raw, exists := args[key]
	if !exists || raw == nil {
		return defaultVal, false, nil
	}

	var intVal int
	switch v := raw.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) {
			return 0, true, fmt.Errorf("参数 %s 必须是有效整数", key)
		}
		if v < float64(math.MinInt) || v > float64(math.MaxInt) {
			return 0, true, fmt.Errorf("参数 %s 超出整数范围", key)
		}
		intVal = int(v)
	case int:
		intVal = v
	case int64:
		if v < int64(math.MinInt) || v > int64(math.MaxInt) {
			return 0, true, fmt.Errorf("参数 %s 超出整数范围", key)
		}
		intVal = int(v)
	default:
		return 0, true, fmt.Errorf("参数 %s 必须是整数", key)
	}

	if intVal < minVal {
		return 0, true, fmt.Errorf("参数 %s 不能小于 %d", key, minVal)
	}
	if intVal > maxVal {
		if clampMax {
			return maxVal, true, nil
		}
		return 0, true, fmt.Errorf("参数 %s 不能大于 %d", key, maxVal)
	}
	return intVal, true, nil
}

func parseRequiredPositiveUint(args map[string]interface{}, key string) (uint, error) {
	if args == nil {
		return 0, fmt.Errorf("缺少参数 %s", key)
	}
	raw, exists := args[key]
	if !exists || raw == nil {
		return 0, fmt.Errorf("缺少参数 %s", key)
	}

	switch v := raw.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > float64(math.MaxUint32) || v != math.Trunc(v) {
			return 0, fmt.Errorf("参数 %s 无效", key)
		}
		return uint(v), nil
	case int:
		if v <= 0 {
			return 0, fmt.Errorf("参数 %s 无效", key)
		}
		return uint(v), nil
	case int64:
		if v <= 0 || v > math.MaxUint32 {
			return 0, fmt.Errorf("参数 %s 无效", key)
		}
		return uint(v), nil
	case uint:
		if v == 0 {
			return 0, fmt.Errorf("参数 %s 无效", key)
		}
		return v, nil
	default:
		return 0, fmt.Errorf("参数 %s 必须是正整数", key)
	}
}
