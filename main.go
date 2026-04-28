package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
	"github.com/xiaoyueRX/Ani-Go/internal/scheduler"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

var version = "dev"

func main() {
	printBanner()

	// 加载配置
	cfg := config.Load()
	log.Printf("配置加载完成 | 端口: %d | 数据库: %s", cfg.Server.Port, cfg.Database.Path)

	// 初始化数据库
	if err := database.Init(cfg.Database.Path); err != nil {
		log.Fatalf("❌ 数据库初始化失败: %v", err)
	}

	printConfig(cfg)

	// 初始化事件总线
	bus := event.New()
	log.Println("✅ 事件总线已初始化")

	// 初始化 Mikan 资源源
	mikanSource := source.NewMikanSource(cfg.Mikan.Domain, cfg.Mikan.ProxyDomain)
	log.Printf("✅ Mikan 资源源已就绪 (域名: %s)", cfg.Mikan.Domain)

	// 初始化 qBittorrent 下载器
	if cfg.Downloaders.QBittorrent.Enabled {
		log.Printf("✅ qBittorrent 下载器已就绪 (地址: %s)", cfg.Downloaders.QBittorrent.Host)
	} else {
		log.Println("⚠️  qBittorrent 未启用，下载功能不可用")
	}

	qb := downloader.NewQBittorrent(
		cfg.Downloaders.QBittorrent.Host,
		cfg.Downloaders.QBittorrent.Username,
		cfg.Downloaders.QBittorrent.Password,
		cfg.Downloaders.QBittorrent.Category,
	)

	// 初始化文件整理器
	org := organizer.New(
		cfg.Organizer.TVTemplate,
		cfg.Organizer.MovieTemplate,
		cfg.Organizer.TVBasePath,
		cfg.Organizer.MovieBasePath,
		cfg.Organizer.UseHardLink,
	)
	log.Println("✅ 文件整理器已就绪")

	// 监听下载完成事件，自动触发文件整理
	bus.Subscribe("download.completed", func(event core.Event) {
		log.Printf("📢 收到下载完成事件: %v", event)
		// TODO: 读取 Episode 记录，调用 org.Organize()
	})

	// 启动调度器
	sched := scheduler.New(cfg, mikanSource, qb, org, bus)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	fmt.Println("\n✅ Ani-Go 启动成功 — Phase 1 核心引擎运行中")
	fmt.Println("   定时任务: RSS 轮询 | 文件整理")
	fmt.Println("   按 Ctrl+C 退出")

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n👋 Ani-Go 正在关闭...")
	cancel()
}

func printBanner() {
	fmt.Println(`
	   _          _        ____
	  / \   _ __ (_)      / ___| ___
	 / _ \ | '_ \| |_____| |  _ / _ \
	/ ___ \| | | | |_____| |_| | (_) |
       /_/   \_\_| |_|_|      \____|\___|

	Ani-Go - 全自动番剧追番管理系统  v` + version + `
	`)
}

func printConfig(cfg *config.Config) {
	fmt.Println("━━━━━━━━━━━━━━━━━ 当前配置 ━━━━━━━━━━━━━━━━━")
	if cfg.Mikan.PersonalRSSURL != "" {
		fmt.Println("✅ Mikan 个人 RSS: 已配置")
	} else {
		fmt.Println("⚠️  Mikan 个人 RSS: 未配置")
		fmt.Println("   设置方法: export MIKAN_RSS_URL=\"https://mikanani.me/RSS/MyBangumi?token=YOUR_TOKEN\"")
	}
	if cfg.Downloaders.QBittorrent.Enabled {
		fmt.Printf("✅ qBittorrent: %s\n", cfg.Downloaders.QBittorrent.Host)
	} else {
		fmt.Println("⚠️  qBittorrent: 未配置")
		fmt.Println("   设置方法: export QB_HOST=http://localhost:8081 && export QB_USER=admin && export QB_PASS=password")
	}
	fmt.Printf("   番剧目录: %s\n", cfg.Organizer.TVBasePath)
	fmt.Printf("   RSS 间隔: %v | 整理间隔: %v\n", cfg.Scheduler.RSSInterval, cfg.Scheduler.OrganizerInterval)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
