package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/api"
	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
	"github.com/xiaoyueRX/Ani-Go/internal/metadata"
	"github.com/xiaoyueRX/Ani-Go/internal/notifier"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
	parsepkg "github.com/xiaoyueRX/Ani-Go/internal/parser"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
	"github.com/xiaoyueRX/Ani-Go/internal/scheduler"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

var version = "dev"

func main() {
	printBanner()

	// 加载配置
	cfg := config.Load()
	log.Printf("配置加载完成 | 端口: %d | 数据库: %s", cfg.Server.Port, cfg.Database.Path)

	// 初始化 JWT 动态密钥（crypto/rand 生成，绝不硬编码）
	if err := auth.InitSecret(); err != nil {
		log.Fatalf("❌ JWT Secret 初始化失败: %v", err)
	}
	log.Println("✅ JWT 动态密钥已生成 (crypto/rand 32B)")

	// 初始化数据库
	if err := database.Init(cfg.Database.Path); err != nil {
		log.Fatalf("❌ 数据库初始化失败: %v", err)
	}

	// 自动创建默认管理员 admin/admin（Bcrypt 哈希存储）
	if err := database.InitDefaultUser(auth.HashPassword); err != nil {
		log.Fatalf("❌ 默认用户创建失败: %v", err)
	}

	// 从数据库设置表合并 Web UI 中保存的配置（env var 优先，数据库作回退）
	cfg.MergeFromSettings(func(key string) (string, bool) {
		var s database.Setting
		res := database.DB.Where("key = ?", key).Limit(1).Find(&s)
		if res.RowsAffected == 0 {
			return "", false
		}
		return s.Value, true
	})

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

	// 初始化 Mikan 资源源（主源，个人 RSS 订阅）
	mikanSource := source.NewMikanSource(cfg.Mikan.Domain, cfg.Mikan.ProxyDomain, cfg.Mikan.MirrorDomains)
	log.Printf("✅ Mikan 资源源已就绪 (域名: %s, 镜像: %d 个)", cfg.Mikan.Domain, len(cfg.Mikan.MirrorDomains))

	// 初始化多资源站聚合器（用于补全搜索）
	extraSources := make([]core.Source, 0, 3)
	if cfg.Sources.Nyaa.Enabled {
		nyaaSrc := source.NewNyaaSource(cfg.Sources.Nyaa.Domain)
		extraSources = append(extraSources, nyaaSrc)
		log.Printf("✅ Nyaa 资源源已就绪 (域名: %s)", cfg.Sources.Nyaa.Domain)
	}
	if cfg.Sources.ACGRIP.Enabled {
		acgripSrc := source.NewACGRIPSource(cfg.Sources.ACGRIP.Domain)
		extraSources = append(extraSources, acgripSrc)
		log.Printf("✅ ACG.RIP 资源源已就绪 (域名: %s)", cfg.Sources.ACGRIP.Domain)
	}
	if cfg.Sources.AnimeTosho.Enabled {
		atSrc := source.NewAnimeToshoSource(cfg.Sources.AnimeTosho.Domain)
		extraSources = append(extraSources, atSrc)
		log.Printf("✅ AnimeTosho 资源源已就绪 (域名: %s)", cfg.Sources.AnimeTosho.Domain)
	}
	multiSource := source.NewMultiSource(append([]core.Source{mikanSource}, extraSources...)...)

	if len(extraSources) == 0 {
		log.Println("ℹ️  未启用额外资源站（仅使用 Mikan）")
	}

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

	// 初始化文件整理器
	org := organizer.New(
		cfg.Organizer.TVTemplate,
		cfg.Organizer.MovieTemplate,
		cfg.Organizer.TVBasePath,
		cfg.Organizer.MovieBasePath,
		cfg.Organizer.UseHardLink,
	)
	log.Println("✅ 文件整理器已就绪")

	// 初始化 TMDB 元数据提供者（可选）
	var primaryMetadata core.MetadataProvider
	if cfg.Metadata.TMDB.Enabled && cfg.Metadata.TMDB.APIKey != "" {
		primaryMetadata = metadata.NewTMDBProvider(
			cfg.Metadata.TMDB.APIKey,
			cfg.Metadata.TMDB.Language,
			cfg.Metadata.TMDB.MirrorDomains,
		)
		log.Printf("✅ TMDB 元数据提供者已就绪 (语言: %s)", cfg.Metadata.TMDB.Language)
	}

	// 初始化 BGM.tv 元数据提供者（可选）
	if cfg.Metadata.BGMTV.Enabled && cfg.Metadata.BGMTV.UserToken != "" {
		bgmProvider := metadata.NewBGMTVProvider(
			cfg.Metadata.BGMTV.UserToken,
			cfg.Metadata.BGMTV.MirrorDomains,
		)
		log.Printf("✅ BGM.tv 元数据提供者已就绪")
		// 若 BGM 被设为主元数据源则优先使用
		if cfg.Metadata.Primary == "bgmtv" {
			primaryMetadata = bgmProvider
		}
	}

	// 初始化 AI 客户端（可选，支持 OpenAI / Google / Anthropic / Ollama / 通用协议）
	var aiClient ai.Classifier
	if cfg.AI.Enabled {
		endpoint, apiKey, model := resolveAIConfig(cfg)
		protocol := ai.Protocol(cfg.AI.Protocol)
		if protocol == "" || protocol == "auto" {
			aiClient = ai.NewClient(endpoint, apiKey, model)
		} else {
			aiClient = ai.NewClientWithProtocol(endpoint, apiKey, model, protocol)
		}
		if aiClient.IsAvailable(context.Background()) {
			protoStr := cfg.AI.Protocol
			if protoStr == "" {
				protoStr = "auto"
			}
			log.Printf("🤖 AI 辅助模块已就绪 (协议: %s | 模型: %s)", protoStr, model)
		} else {
			log.Println("⚠️  AI 模块已启用但未配置凭证/端点")
		}
	} else {
		log.Println("ℹ️  AI 模块未启用，核心功能不受影响")
	}

	// 监听下载完成事件
	bus.Subscribe("download.completed", func(event core.Event) {
		log.Printf("📢 收到下载完成事件: %v", event)
	})

	// 初始化插件管理器
	pluginMgr := plugin.NewManager(bus)
	pluginMgr.LoadFromSettings()
	pluginMgr.SubscribeAll()

	// 初始化通知系统（可选，支持 Telegram/Discord/企业微信/飞书/钉钉）
	mn := setupNotifier(cfg)
	if mn.Count() > 0 {
		log.Printf("🔔 已启用 %d 个通知渠道", mn.Count())
		// 订阅关键事件以自动推送通知
		bus.Subscribe(core.EventDownloadStarted, func(event core.Event) {
			title, _ := event.Payload["title"].(string)
			mn.Send(context.Background(), "⬇️ 下载开始", title)
		})
		bus.Subscribe(core.EventDownloadCompleted, func(event core.Event) {
			title, _ := event.Payload["title"].(string)
			mn.Send(context.Background(), "✅ 下载完成", title)
		})
		bus.Subscribe(core.EventDownloadFailed, func(event core.Event) {
			title, _ := event.Payload["title"].(string)
			mn.Send(context.Background(), "❌ 下载失败", title)
		})
		bus.Subscribe(core.EventSupplementCompleted, func(event core.Event) {
			title, _ := event.Payload["title"].(string)
			mn.Send(context.Background(), "📦 补全完成", title)
		})
	} else {
		log.Println("ℹ️  未启用通知渠道")
	}

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
	if err := api.StartServer(ctx, cfg.Server.Host, cfg.Server.Port, dl, sched.TriggerSupplement, pluginMgr, taskParser, mikanSource, multiSource, staticHandler()); err != nil {
		log.Fatalf("❌ HTTP API 服务启动失败: %v", err)
	}

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
