// AutoAni - 全自动番剧追番下载管理系统
//
// 作者: xiaoyueRX
// 协议: MIT License
//
// 项目目标：
//   - 自动追踪 Mikan 个人 RSS 中的新番
//   - 支持历史全量补全（老番缺集自动找种）
//   - 多下载器支持（qBittorrent / Transmission / Aria2）
//   - 文件自动整理（Jellyfin/fnOS 兼容命名格式）
//   - 高度可扩展（接口驱动架构，插件系统）
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/xiaoyueRX/autoani/internal/config"
	"github.com/xiaoyueRX/autoani/internal/database"
)

// version 是程序版本号，构建时通过 -ldflags 注入
// 例：go build -ldflags="-X main.version=0.1.0"
var version = "dev"

func main() {
	// ——— 打印启动横幅 ———
	printBanner()

	// ——— 第一步：加载配置 ———
	// 从环境变量读取所有配置（Docker 部署时通过 -e 注入）
	cfg := config.Load()
	log.Printf("配置加载完成 | 端口: %d | 数据库: %s", cfg.Server.Port, cfg.Database.Path)

	// ——— 第二步：初始化数据库 ———
	// 自动创建 SQLite 数据库文件和数据表
	if err := database.Init(cfg.Database.Path); err != nil {
		log.Fatalf("❌ 数据库初始化失败: %v", err)
		os.Exit(1)
	}

	// ——— 打印当前配置摘要 ———
	printConfig(cfg)

	// TODO: 后续步骤将在这里逐步添加：
	// - 初始化资源站（Mikan Source）
	// - 初始化下载器（qBittorrent）
	// - 初始化元数据客户端（TMDB / BGM.tv）
	// - 启动定时任务（RSS 轮询 / 补全检查）
	// - 启动 Web API 服务器

	fmt.Println("\n✅ AutoAni 启动成功，等待功能模块接入...")
	fmt.Println("   下一步：实现 Mikan RSS 解析器")
	fmt.Println("   按 Ctrl+C 退出")

	// 等待系统退出信号（Ctrl+C 或 kill）
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // 阻塞在这里，直到收到退出信号

	fmt.Println("\n👋 AutoAni 正在关闭...")
}

func printBanner() {
	fmt.Println(`
  ___        _        _             _ 
 / _ \      | |      / \           (_)
/ /_\ \_   _| |_ ___/ _ \  ___  _  _ 
|  _  | | | | __/ _ \ | || '_ \| || |
| | | | |_| | || (_) | |_| | | | || |
\_| |_/\__,_|\__\___/\___/_| |_|_|_|_|

AutoAni - 全自动番剧追番管理系统
`)
}

func printConfig(cfg *config.Config) {
	fmt.Println("━━━━━━━━━━━━━━━━━ 当前配置 ━━━━━━━━━━━━━━━━━")

	// Mikan 配置
	if cfg.Mikan.PersonalRSSURL != "" {
		fmt.Println("✅ Mikan 个人 RSS: 已配置")
	} else {
		fmt.Println("⚠️  Mikan 个人 RSS: 未配置（需设置环境变量 MIKAN_RSS_URL）")
	}

	// 下载器
	if cfg.Downloaders.QBittorrent.Enabled {
		fmt.Printf("✅ qBittorrent: %s\n", cfg.Downloaders.QBittorrent.Host)
	} else {
		fmt.Println("⚠️  qBittorrent: 未配置")
	}

	// 元数据
	if cfg.Metadata.TMDB.Enabled {
		fmt.Println("✅ TMDB 元数据: 已配置")
	} else {
		fmt.Println("   TMDB 元数据: 未配置（将使用 BGM.tv）")
	}

	// AI
	if cfg.AI.Enabled {
		fmt.Printf("✅ AI 辅助: 已启用 (模型: %s)\n", cfg.AI.Model)
	} else {
		fmt.Println("   AI 辅助: 未启用（纯规则模式）")
	}

	// 路径
	fmt.Printf("   番剧目录: %s\n", cfg.Organizer.TVBasePath)
	fmt.Printf("   剧场版目录: %s\n", cfg.Organizer.MovieBasePath)

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
