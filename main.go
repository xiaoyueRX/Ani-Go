package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/xiaoyueRX/Ani-rss/internal/config"
	"github.com/xiaoyueRX/Ani-rss/internal/database"
)

var version = "dev"

func main() {
	printBanner()

	cfg := config.Load()
	log.Printf("配置加载完成 | 端口: %d | 数据库: %s", cfg.Server.Port, cfg.Database.Path)

	if err := database.Init(cfg.Database.Path); err != nil {
		log.Fatalf("❌ 数据库初始化失败: %v", err)
		os.Exit(1)
	}

	printConfig(cfg)

	fmt.Println("\n✅ Ani-Go 启动成功，等待功能模块接入...")
	fmt.Println("   下一步：实现 Mikan RSS 解析器")
	fmt.Println("   按 Ctrl+C 退出")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n👋 Ani-Go 正在关闭...")
}

func printBanner() {
	fmt.Println(`
    ___         _                    
   /   |  ____ (_)       __________ _____
  / /| | / __ \/ /______/ ___/ ___// ___/
 / ___ |/ / / / /______/ /  (__  )/__  ) 
/_/  |_/_/ /_/_/      /_/  /____//____/  
                                         
Ani-Go - 全自动番剧追番管理系统
`)
}

func printConfig(cfg *config.Config) {
	fmt.Println("━━━━━━━━━━━━━━━━━ 当前配置 ━━━━━━━━━━━━━━━━━")
	if cfg.Mikan.PersonalRSSURL != "" {
		fmt.Println("✅ Mikan 个人 RSS: 已配置")
	} else {
		fmt.Println("⚠️  Mikan 个人 RSS: 未配置")
	}
	if cfg.Downloaders.QBittorrent.Enabled {
		fmt.Printf("✅ qBittorrent: %s\n", cfg.Downloaders.QBittorrent.Host)
	} else {
		fmt.Println("⚠️  qBittorrent: 未配置")
	}
	fmt.Printf("   番剧目录: %s\n", cfg.Organizer.TVBasePath)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
