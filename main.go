package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

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
	"github.com/xiaoyueRX/Ani-Go/internal/update"
)

var version = "v0.7.0"

func main() {
	if update.RunHandoff(os.Args[1:]) {
		return
	}
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
		if err = os.MkdirAll(filepath.Dir(cfg.Server.LogPath), 0700); err == nil {
			logFile, err = os.OpenFile(cfg.Server.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		}
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

	// 初次创建管理员并使用 Bcrypt 存储口令。
	if err := database.InitDefaultUser(auth.HashPassword); err != nil {
		log.Fatalf("❌ 默认用户创建失败: %v", err)
	}

	// 初始化 JWT 密钥（从数据库加载或生成新密钥并持久化，重启不丢失）
	if err := auth.InitSecretFromDB(database.DB); err != nil {
		log.Fatalf("❌ JWT Secret 初始化失败: %v", err)
	}
	log.Println("✅ JWT 密钥已从数据库加载或自动生成 (持久化存储)")

	// 从数据库设置表批量合并 Web UI 中保存的配置（env var 优先，数据库作回退）
	startupDefaults := *cfg
	startupSettings := database.LoadSettingsMap(database.DB)
	getStartupSetting := func(key string) (string, bool) {
		v, ok := startupSettings[key]
		return v, ok
	}
	cfg.MergeFromSettings(getStartupSetting)

	// 初始化默认设置（仅当 stall_timeout_hours 键不存在时写入，避免与用户已配置的值冲突）
	if _, ok := startupSettings["stall_timeout_hours"]; !ok {
		database.DB.Create(&database.Setting{Key: "stall_timeout_hours", Value: "24"})
	}

	printConfig(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	// 初始化事件总线
	bus := event.New()
	log.Println("✅ 事件总线已初始化")

	// 初始化系统自更新管理器
	updateMgr := update.InitManager(version)
	updateMgr.SetListenAddress(cfg.Server.Host, cfg.Server.Port)
	updateDone := make(chan struct{})
	go func() {
		defer close(updateDone)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		if info, err := updateMgr.CheckUpdate(ctx); err == nil && info.HasUpdate {
			log.Printf("✨ [版本检测] 发现云端新版本: %s (当前: %s) | 支持在 Web 界面一键平滑升级", info.LatestVersion, info.CurrentVersion)
		}
	}()

	// 从已批量读取的数据库设置加载用户自定义正则解析规则
	source.LoadCustomPatternsFromSettings(getStartupSetting)

	// 初始化资源源
	sourceConfig := *cfg
	sourceConfig.Sources = startupDefaults.Sources
	mikanSource, yucSource, multiSource := app.InitSources(&sourceConfig)
	log.Println("✅ 资源站初始化完成")

	// 根据配置选择默认下载器（支持 qBittorrent / Transmission）
	dl := downloader.NewConfigured(cfg.Downloaders)
	log.Printf("⬇️ 当前下载器: %s", dl.Name())

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
	serviceCtx, stopServices := context.WithCancel(context.WithoutCancel(ctx))
	defer stopServices()
	services := core.NewContext(serviceCtx)
	pluginMgr := plugin.NewManager(bus, services)

	// 初始化文件整理器
	org := organizer.New(
		cfg.Organizer.TVTemplate,
		cfg.Organizer.MovieTemplate,
		cfg.Organizer.OtherTemplate,
		cfg.Organizer.TVBasePath,
		cfg.Organizer.MovieBasePath,
		cfg.Organizer.UseHardLink,
		pluginMgr,
		cfg.Organizer.OVABasePath,
	)
	log.Println("✅ 文件整理器已就绪")

	// 初始化通知系统 v2（支持全部 16 种通道统一队列分发与重试）
	notifyMgr := v2.NewNotifyManager(nil, bus)
	notifyMgr.SetReloader(notifier.BuildAllNotifiersFromSettingsAndEnv)
	// 绑定通知中心与消息通知推送插件的生命周期
	plugin.DefaultExtendedNotifiersPlugin.SetController(notifyMgr)
	defer notifyMgr.Stop()

	dynamicDL := downloader.NewDynamicDownloader(dl)

	// 启动调度器
	sched := scheduler.New(cfg, multiSource, dynamicDL, org, bus, primaryMetadata, aiClient, &startupDefaults)

	// 初始化自然语言任务解析器（正则 + AI 回退）
	var taskParser core.TaskParser
	if aiClient != nil {
		taskParser = parsepkg.NewCompositeParser(aiClient)
		log.Println("🧠 任务解析器已就绪（正则 + AI 回退）")
	} else {
		taskParser = parsepkg.NewCompositeParser(nil)
		log.Println("🧠 任务解析器已就绪（仅正则模式）")
	}

	// 在装载插件前，发布实际由本进程使用的服务实例。
	services.SetService(core.ServiceEventBus, bus)
	services.SetService(core.ServiceConfig, cfg)
	services.SetService(core.ServiceDB, database.DB)
	services.SetService(core.ServiceSource, multiSource)
	services.SetService(core.ServiceMetadata, primaryMetadata)
	if aiClient != nil {
		services.SetService(core.ServiceAI, app.PluginAI(aiClient, cfg.AI.Protocol))
	}
	services.SetService(core.ServiceDownloader, dynamicDL)
	services.SetService(core.ServiceOrganizer, org)
	services.SetService(core.ServiceNotifier, notifyMgr)
	services.SetService(core.ServiceScheduler, sched)
	services.SetService(core.ServiceBackupManager, sched.GetBackupManager())
	services.SetService(core.ServiceTaskParser, taskParser)
	if err := pluginMgr.Load(); err != nil {
		log.Printf("插件配置暂时不可用，可在插件中心重新加载: %v", err)
	}
	// 备份仅接受本次构建实际内置的面板和快照中声明的社区授权。
	sched.GetBackupManager().SetBuiltinPanelManifests(pluginMgr.GetPluginList())
	if notifyMgr.IsEnabled() {
		var names []string
		for _, notifier := range notifyMgr.Notifiers() {
			names = append(names, notifier.Name())
		}
		if len(names) > 0 {
			log.Printf("🔔 已配置 %d 个通知渠道: %v", len(names), names)
		}
	}
	schedulerDone := sched.StartAsync(ctx)

	// 启动 HTTP API 服务（含 JWT 鉴权中间件 + 嵌入式前端静态文件）
	httpStopped := make(chan struct{})
	api.StartServer(ctx, cfg.Server.Host, cfg.Server.Port, version, cfg.Server.AllowedOrigins, dynamicDL, sched.TriggerSupplement, pluginMgr, taskParser, mikanSource.(*source.MikanSource), yucSource.(*source.YucWikiSource), multiSource, staticHandler(), cfg.Server.LogPath, notifyMgr, primaryMetadata, bus, api.ServerOptions{
		Stopped:            httpStopped,
		ReloadIntervals:    sched.ReloadIntervals,
		SmartSearchEnabled: cfg.AI.SmartSearchEnabled,
		AIChat:             aiClient,
		BackupManager:      sched.GetBackupManager(),
		Organizer:          org,
	})

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("✅ Ani-Go 启动成功 — Phase 3 全栈引擎运行中")
	address := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	fmt.Printf("   Web UI: http://%s\n", address)
	fmt.Printf("   API: http://%s/api/login\n", address)
	fmt.Println("   前端已嵌入二进制 (go:embed) | 单文件部署")
	fmt.Println("   定时任务: RSS 轮询 | 文件整理")
	fmt.Println("   按 Ctrl+C 退出")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 等待退出信号
	<-ctx.Done()

	fmt.Println("\n👋 Ani-Go 正在关闭...")
	cancel()
	<-updateDone
	<-schedulerDone
	<-httpStopped
	drainCtx, stopDrain := context.WithTimeout(context.Background(), 5*time.Second)
	if err := bus.Close(drainCtx); err != nil {
		log.Printf("事件总线未在关停期限内排空: %v", err)
	}
	stopDrain()
	pluginMgr.StopAll()
	stopServices()
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

// reloadNotifiersV2 从配置与数据库创建全渠道通知器并热加载到 NotifyManager
func reloadNotifiersV2(cfg *config.Config, mgr *v2.NotifyManager) {
	notifiers := notifier.BuildAllNotifiersFromSettingsAndEnv()
	mgr.ReloadNotifiers(notifiers)
}
