package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/xiaoyueRX/Ani-Go/internal/api"
	"github.com/xiaoyueRX/Ani-Go/internal/app"
	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
	"github.com/xiaoyueRX/Ani-Go/internal/notifier"
	"github.com/xiaoyueRX/Ani-Go/internal/notifier/v2"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
	parsepkg "github.com/xiaoyueRX/Ani-Go/internal/parser"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
	"github.com/xiaoyueRX/Ani-Go/internal/scheduler"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

var version = "v0.5.0"

func main() {
	printBanner()

	// 加载 .env
	if err := godotenv.Load(); err != nil {
		log.Println("ℹ️  未找到 .env 文件，将使用系统环境变量")
	}

	// 加载配置
	cfg := config.Load()
	log.Printf("配置加载完成 | 端口: %d | 数据库: %s", cfg.Server.Port, cfg.Database.Path)

	// 初始化日志文件（双写 stdout + 文件）
	var logFile *os.File
	if cfg.Server.LogPath != "" {
		var err error
		logFile, err = os.OpenFile(cfg.Server.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			log.SetOutput(io.MultiWriter(os.Stdout, logFile))
		} else {
			log.Printf("⚠️ 日志文件 %s 创建失败: %v", cfg.Server.LogPath, err)
		}
	}

	// 初始化数据库
	if err := database.Init(cfg.Database.Path); err != nil {
		log.Fatalf("❌ 数据库初始化失败: %v", err)
	}

	// 自动创建默认管理员 admin/admin（Bcrypt 哈希存储）
	if err := database.InitDefaultUser(auth.HashPassword); err != nil {
		log.Fatalf("❌ 默认用户创建失败: %v", err)
	}

	// 初始化 JWT 密钥（从数据库加载或生成新密钥并持久化，重启不丢失）
	if err := auth.InitSecretFromDB(
		func(key string) (string, bool) {
			var s database.Setting
			if err := database.DB.Where("key = ?", key).First(&s).Error; err != nil {
				return "", false
			}
			return s.Value, true
		},
		func(key, value string) {
			database.DB.Save(&database.Setting{Key: key, Value: value})
		},
	); err != nil {
		log.Fatalf("❌ JWT Secret 初始化失败: %v", err)
	}
	log.Println("✅ JWT 密钥已从数据库加载或自动生成 (持久化存储)")

	// 从数据库设置表合并 Web UI 中保存的配置（env var 优先，数据库作回退）
	cfg.MergeFromSettings(func(key string) (string, bool) {
		var s database.Setting
		res := database.DB.Where("key = ?", key).Limit(1).Find(&s)
		if res.RowsAffected == 0 {
			return "", false
		}
		return s.Value, true
	})

	// 初始化默认设置（仅当 stall_timeout_hours 键不存在时写入，避免与用户已配置的值冲突）
	var stallSetting database.Setting
	if err := database.DB.Where("key = ?", "stall_timeout_hours").First(&stallSetting).Error; err != nil {
		database.DB.Create(&database.Setting{Key: "stall_timeout_hours", Value: "24"})
	}

	printConfig(cfg)

	// 初始化事件总线
	bus := event.New()
	log.Println("✅ 事件总线已初始化")

	// 从数据库加载用户自定义正则解析规则
	source.LoadCustomPatternsFromSettings(func(key string) (string, bool) {
		var s database.Setting
		if err := database.DB.Where("key = ?", key).First(&s).Error; err != nil {
			return "", false
		}
		return s.Value, true
	})

	// 初始化资源源
	mikanSource, yucSource, multiSource := app.InitSources(cfg)
	log.Println("✅ 资源站初始化完成")

	// 根据配置选择默认下载器（支持 qBittorrent / Transmission）
	var dl core.Downloader
	switch cfg.Downloaders.Default {
	case "transmission":
		dl = downloader.NewTransmission(
			cfg.Downloaders.Transmission.Host,
			cfg.Downloaders.Transmission.Username,
			cfg.Downloaders.Transmission.Password,
		)
		if cfg.Downloaders.Transmission.Enabled {
			log.Printf("✅ Transmission 下载器已就绪 (地址: %s)", cfg.Downloaders.Transmission.Host)
		} else {
			log.Println("⚠️  Transmission 未配置，下载功能不可用")
		}
	case "aria2":
		dl = downloader.NewAria2(
			cfg.Downloaders.Aria2.Host,
			cfg.Downloaders.Aria2.Secret,
		)
		if cfg.Downloaders.Aria2.Enabled {
			log.Printf("✅ Aria2 下载器已就绪 (地址: %s)", cfg.Downloaders.Aria2.Host)
		} else {
			log.Println("⚠️  Aria2 未配置，下载功能不可用")
		}
	default:
		qb := downloader.NewQBittorrent(
			cfg.Downloaders.QBittorrent.Host,
			cfg.Downloaders.QBittorrent.Username,
			cfg.Downloaders.QBittorrent.Password,
			cfg.Downloaders.QBittorrent.Category,
		)
		dl = qb
		if cfg.Downloaders.QBittorrent.Enabled {
			log.Printf("✅ qBittorrent 下载器已就绪 (地址: %s)", cfg.Downloaders.QBittorrent.Host)
		} else {
			log.Println("⚠️  qBittorrent 未配置，下载功能不可用")
		}
	}

	// 初始化元数据提供者
	primaryMetadata := app.InitMetadata(cfg)
	log.Println("✅ 元数据提供者初始化完成")

	// 初始化 AI 客户端
	aiClient := app.InitAI(cfg, resolveAIConfig)
	if aiClient != nil {
		log.Println("🤖 AI 辅助模块已就绪")
	}

	// 监听下载完成事件
	bus.Subscribe("download.completed", func(event core.Event) {
		log.Printf("📢 收到下载完成事件: %v", event)
	})

	// 初始化插件管理器
	pluginMgr := plugin.NewManager(bus)
	pluginMgr.Load()

	// 初始化文件整理器
	org := organizer.New(
		cfg.Organizer.TVTemplate,
		cfg.Organizer.MovieTemplate,
		cfg.Organizer.OtherTemplate,
		cfg.Organizer.TVBasePath,
		cfg.Organizer.MovieBasePath,
		cfg.Organizer.UseHardLink,
		pluginMgr,
	)
	// 极客命名插件为可选功能，默认关闭以避免改变既有媒体库路径；
	// 仅当显式设置 ANIGO_GEEK_NAMING=1 时启用
	if os.Getenv("ANIGO_GEEK_NAMING") == "1" {
		plugin.InitGeekNaming(org.GetHookManager())
		log.Println("🧪 极客命名插件已启用 (ANIGO_GEEK_NAMING=1)")
	}
	log.Println("✅ 文件整理器已就绪")

	// 初始化通知系统 v2（支持 Telegram/钉钉/企业微信/飞书/QQ OneBot）
	notifyMgr := v2.NewNotifyManager(nil, bus)
	// 从配置创建通知器并热加载
	reloadNotifiersV2(cfg, notifyMgr)
	if len(notifyMgr.GetStats()) > 0 || true { // 统计暂时为空，用 provider 名称判断
		names := []string{}
		for _, n := range notifyMgr.Notifiers() {
			names = append(names, n.Name())
		}
		if len(names) > 0 {
			log.Printf("🔔 已启用 %d 个通知渠道: %v", len(names), names)
		}
	}
	notifyMgr.Start()
	defer notifyMgr.Stop()

	// 启动调度器
	sched := scheduler.New(cfg, multiSource, dl, org, bus, primaryMetadata, aiClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	// 初始化自然语言任务解析器（正则 + AI 回退）
	var taskParser core.TaskParser
	if aiClient != nil {
		taskParser = parsepkg.NewCompositeParser(aiClient)
		log.Println("🧠 任务解析器已就绪（正则 + AI 回退）")
	} else {
		taskParser = parsepkg.NewCompositeParser(nil)
		log.Println("🧠 任务解析器已就绪（仅正则模式）")
	}

	// 启动 HTTP API 服务（含 JWT 鉴权中间件 + 嵌入式前端静态文件）
	api.StartServer(ctx, cfg.Server.Host, cfg.Server.Port, version, cfg.Server.AllowedOrigins, dl, sched.TriggerSupplement, pluginMgr, taskParser, mikanSource.(*source.MikanSource), yucSource.(*source.YucWikiSource), multiSource, staticHandler(), cfg.Server.LogPath, notifyMgr, primaryMetadata, bus, api.ServerOptions{
		SmartSearchEnabled: cfg.AI.SmartSearchEnabled,
		AIChat:             aiClient,
		BackupManager:      sched.GetBackupManager(),
	})

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("✅ Ani-Go 启动成功 — Phase 3 全栈引擎运行中")
	fmt.Printf("   Web UI: http://%s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("   API: http://%s:%d/api/login\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Println("   默认账号: admin / admin")
	fmt.Println("   前端已嵌入二进制 (go:embed) | 单文件部署")
	fmt.Println("   定时任务: RSS 轮询 | 文件整理")
	fmt.Println("   按 Ctrl+C 退出")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n👋 Ani-Go 正在关闭...")
	cancel()
	if logFile != nil {
		logFile.Close()
	}
}

// resolveAIConfig 根据配置推断实际的 endpoint、apiKey、model
func resolveAIConfig(cfg *config.Config) (endpoint, apiKey, model string) {
	endpoint = cfg.AI.Endpoint
	apiKey = cfg.AI.APIKey
	model = cfg.AI.Model

	switch cfg.AI.Protocol {
	case "google":
		if cfg.AI.GeminiKey != "" {
			apiKey = cfg.AI.GeminiKey
		}
		if model == "" {
			model = "gemini-2.0-flash"
		}
	case "ollama":
		if cfg.AI.OllamaHost != "" {
			endpoint = cfg.AI.OllamaHost
		}
		if cfg.AI.OllamaModel != "" {
			model = cfg.AI.OllamaModel
		}
		if model == "" {
			model = "llama3"
		}
	case "anthropic":
		if cfg.AI.ClaudeKey != "" {
			apiKey = cfg.AI.ClaudeKey
		}
		if model == "" {
			model = "claude-haiku-4-5-20251001"
		}
	}
	return
}

func printBanner() {
	fmt.Print(`
    ╔═══════════════════════════════════════════════════════════════╗
    ║                                                               ║
    ║      ___   _   _      ____                                     ║
    ║     / _ \ | \ | |    / ___|  ___  _ __   ___                   ║
    ║    / /_\ \|  \| |___| |  _  / _ \| '_ \ / _ \                  ║
    ║   / _____ \ |\  |___| |_| || (_) | | | | (_) |                 ║
    ║  /_/     \_\_| \_|    \____| \___/|_| |_|\___/                  ║
    ║                                                               ║
    ║       全自动番剧追番下载管理系统                                 ║
    ║       Auto Anime Subscription & Download Manager               ║
    ║                                                               ║
    ╚═══════════════════════════════════════════════════════════════╝
`)
	fmt.Printf("  Version: %s | Port: 20001 | GitHub: github.com/xiaoyueRX/Ani-Go\n\n", version)
}

func setupNotifier(cfg *config.Config) *notifier.MultiNotifier {
	mn := notifier.NewMultiNotifier()

	if cfg.Notifier.TelegramBotToken != "" && cfg.Notifier.TelegramChatID != "" {
		mn.Add(notifier.NewTelegramNotifier(cfg.Notifier.TelegramBotToken, cfg.Notifier.TelegramChatID))
	}
	if cfg.Notifier.DiscordWebhook != "" {
		mn.Add(notifier.NewWebhookNotifier(cfg.Notifier.DiscordWebhook, notifier.WebhookDiscord))
	}
	if cfg.Notifier.WecomWebhook != "" {
		mn.Add(notifier.NewWebhookNotifier(cfg.Notifier.WecomWebhook, notifier.WebhookWecom))
	}
	if cfg.Notifier.FeishuWebhook != "" {
		mn.Add(notifier.NewWebhookNotifier(cfg.Notifier.FeishuWebhook, notifier.WebhookFeishu))
	}
	if cfg.Notifier.DingTalkWebhook != "" {
		mn.Add(notifier.NewWebhookNotifier(cfg.Notifier.DingTalkWebhook, notifier.WebhookDingTalk))
	}
	if cfg.Notifier.OneBotHost != "" && (cfg.Notifier.OneBotUserID != 0 || cfg.Notifier.OneBotGroupID != 0) {
		mn.Add(notifier.NewOneBotNotifier(cfg.Notifier.OneBotHost, cfg.Notifier.OneBotToken, cfg.Notifier.OneBotUserID, cfg.Notifier.OneBotGroupID))
	}
	if cfg.Notifier.SlackWebhook != "" {
		mn.Add(notifier.NewSlackNotifier(cfg.Notifier.SlackWebhook))
	}
	if cfg.Notifier.MatrixHomeserver != "" && cfg.Notifier.MatrixToken != "" && cfg.Notifier.MatrixRoomID != "" {
		mn.Add(notifier.NewMatrixNotifier(cfg.Notifier.MatrixHomeserver, cfg.Notifier.MatrixToken, cfg.Notifier.MatrixRoomID))
	}
	if cfg.Notifier.ServerChanKey != "" {
		mn.Add(notifier.NewPushNotifier(notifier.PushServerChan, "https://sctapi.ftqq.com/"+cfg.Notifier.ServerChanKey+".send", "", ""))
	}
	if cfg.Notifier.BarkDeviceKey != "" {
		mn.Add(notifier.NewPushNotifier(notifier.PushBark, "https://api.day.app/push", cfg.Notifier.BarkDeviceKey, ""))
	}
	if cfg.Notifier.PushoverToken != "" && cfg.Notifier.PushoverUser != "" {
		mn.Add(notifier.NewPushNotifier(notifier.PushPushover, "https://api.pushover.net/1/messages.json", cfg.Notifier.PushoverToken, cfg.Notifier.PushoverUser))
	}
	if cfg.Notifier.GotifyURL != "" && cfg.Notifier.GotifyToken != "" {
		mn.Add(notifier.NewPushNotifier(notifier.PushGotify, cfg.Notifier.GotifyURL+"/message?token="+cfg.Notifier.GotifyToken, "", ""))
	}
	if cfg.Notifier.NtfyURL != "" {
		mn.Add(notifier.NewPushNotifier(notifier.PushNtfy, cfg.Notifier.NtfyURL, "", ""))
	}
	if cfg.Notifier.EmailSMTPHost != "" && cfg.Notifier.EmailUsername != "" && cfg.Notifier.EmailTo != "" {
		to := strings.Split(cfg.Notifier.EmailTo, ",")
		for i := range to {
			to[i] = strings.TrimSpace(to[i])
		}
		mn.Add(notifier.NewEmailNotifier(cfg.Notifier.EmailSMTPHost, cfg.Notifier.EmailSMTPPort, cfg.Notifier.EmailUsername, cfg.Notifier.EmailPassword, cfg.Notifier.EmailFrom, to))
	}
	if cfg.Notifier.LINEChannelToken != "" && cfg.Notifier.LINEUserID != "" {
		mn.Add(notifier.NewLINENotifier(cfg.Notifier.LINEChannelToken, cfg.Notifier.LINEUserID))
	}
	if cfg.Notifier.WhatsAppPhoneID != "" && cfg.Notifier.WhatsAppToken != "" && cfg.Notifier.WhatsAppTo != "" {
		mn.Add(notifier.NewWhatsAppNotifier(cfg.Notifier.WhatsAppPhoneID, cfg.Notifier.WhatsAppToken, cfg.Notifier.WhatsAppTo))
	}

	// Add Signal
	if os.Getenv("SIGNAL_API_URL") != "" {
		mn.Add(notifier.NewSignalNotifier())
	}

	// Add WeChat Official Account
	if os.Getenv("WECHAT_APP_ID") != "" {
		mn.Add(notifier.NewWeChatNotifier())
	}

	return mn
}

func printConfig(cfg *config.Config) {
	fmt.Println("━━━━━━━━━━━━━━━━━ 当前配置 ━━━━━━━━━━━━━━━━━")
	if cfg.Mikan.PersonalRSSURL != "" {
		fmt.Println("✅ Mikan 个人 RSS: 已配置")
	} else {
		fmt.Println("⚠️  Mikan 个人 RSS: 未配置")
		fmt.Println("   设置方法: export MIKAN_RSS_URL=\"https://mikanani.me/RSS/MyBangumi?token=YOUR_TOKEN\"")
	}
	switch cfg.Downloaders.Default {
	case "transmission":
		if cfg.Downloaders.Transmission.Enabled {
			fmt.Printf("✅ Transmission: %s\n", cfg.Downloaders.Transmission.Host)
		} else {
			fmt.Println("⚠️  Transmission: 未配置")
		}
	case "aria2":
		if cfg.Downloaders.Aria2.Enabled {
			fmt.Printf("✅ Aria2: %s\n", cfg.Downloaders.Aria2.Host)
		} else {
			fmt.Println("⚠️  Aria2: 未配置")
		}
	default:
		if cfg.Downloaders.QBittorrent.Enabled {
			fmt.Printf("✅ qBittorrent: %s\n", cfg.Downloaders.QBittorrent.Host)
		} else {
			fmt.Println("⚠️  qBittorrent: 未配置")
		}
	}
	fmt.Printf("   番剧目录: %s\n", cfg.Organizer.TVBasePath)
	fmt.Printf("   RSS 间隔: %v | 整理间隔: %v\n", cfg.Scheduler.RSSInterval, cfg.Scheduler.OrganizerInterval)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// reloadNotifiersV2 从配置创建通知器并热加载到 NotifyManager
func reloadNotifiersV2(cfg *config.Config, mgr *v2.NotifyManager) {
	notifierCfg := v2.NotifierConfig{
		TelegramBotToken: cfg.Notifier.TelegramBotToken,
		TelegramChatID:   cfg.Notifier.TelegramChatID,
		DingTalkWebhook:  cfg.Notifier.DingTalkWebhook,
		DingTalkSecret:   "", // 暂不支持
		WeComWebhook:     cfg.Notifier.WecomWebhook,
		FeishuWebhook:    cfg.Notifier.FeishuWebhook,
		OneBotHost:       cfg.Notifier.OneBotHost,
		OneBotToken:      cfg.Notifier.OneBotToken,
		OneBotUserID:     cfg.Notifier.OneBotUserID,
		OneBotGroupID:    cfg.Notifier.OneBotGroupID,
	}
	notifiers := v2.CreateNotifiersFromConfig(notifierCfg)
	mgr.ReloadNotifiers(notifiers)
}
