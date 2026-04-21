// Package config 负责加载和管理 AutoAni 的全部配置。
//
// 配置优先级（从高到低）：
//  1. 环境变量（如 AUTOANI_QB_HOST=http://...）
//  2. 配置文件 config.yaml
//  3. 默认值（代码里写死的兜底值）
//
// 这样设计的好处：
//   - Docker 部署时通过环境变量注入敏感信息（密码不进代码）
//   - 日常调试可以用配置文件
//   - 什么都不配也能跑（默认值兜底）
package config

import (
	"os"
	"strconv"
	"time"
)

// Config 是整个程序的配置根结构体
// 结构体 = 把相关数据打包在一起，类似 YAML 的层级
type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	Mikan      MikanConfig
	Downloaders DownloadersConfig
	Metadata   MetadataConfig
	Organizer  OrganizerConfig
	AI         AIConfig
	Scheduler  SchedulerConfig
}

// ServerConfig Web 服务器配置
type ServerConfig struct {
	Host string // 监听地址，默认 "0.0.0.0"
	Port int    // 监听端口，默认 8080
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Path string // SQLite 文件路径，默认 "/data/autoani.db"
}

// MikanConfig Mikan Project 相关配置
type MikanConfig struct {
	// 用户个人 RSS URL（最重要的配置）
	// 在 Mikan 登录后 → 我的番组 → 复制 RSS 链接
	PersonalRSSURL string

	// Mikan 网站域名（支持自建反代）
	// 默认 "mikanani.me"
	Domain string

	// 如果你有自建的 Mikan 反代（比如你的 mikan.金陵十钗.icu）
	// 填这里，留空则用官方域名
	ProxyDomain string
}

// DownloadersConfig 下载器配置
// 支持同时配置多个下载器
type DownloadersConfig struct {
	// 默认使用哪个下载器（填下载器的 Name()，如 "qbittorrent"）
	Default string

	QBittorrent QBittorrentConfig
	Transmission TransmissionConfig
	Aria2        Aria2Config
}

type QBittorrentConfig struct {
	Enabled  bool
	Host     string // 如 "http://127.0.0.1:8080"
	Username string
	Password string
	Category string // 添加种子时使用的分类标签，如 "bangumi"
}

type TransmissionConfig struct {
	Enabled  bool
	Host     string
	Username string
	Password string
}

type Aria2Config struct {
	Enabled bool
	Host    string // 如 "http://127.0.0.1:6800/jsonrpc"
	Secret  string // Aria2 的 RPC secret
}

// MetadataConfig 元数据配置
type MetadataConfig struct {
	// 优先使用哪个元数据源（"tmdb" 或 "bgmtv"）
	Primary string

	TMDB  TMDBConfig
	BGMTV BGMTVConfig
}

type TMDBConfig struct {
	Enabled bool
	APIKey  string
	// 语言偏好，影响返回的标题语言
	Language string // 如 "zh-CN"
}

type BGMTVConfig struct {
	Enabled  bool
	// BGM.tv 不需要 API Key，但可选填用户 Token 以提高限速
	UserToken string
}

// OrganizerConfig 文件整理配置
type OrganizerConfig struct {
	// 各类型媒体的根目录
	TVBasePath    string // 电视剧/番剧根目录，如 "/TV/Media/番剧"
	MovieBasePath string // 剧场版根目录，如 "/TV/Media/剧场版"
	OVABasePath   string // OVA 根目录（留空=并入番剧目录的 Specials）

	// 路径模板（支持变量替换）
	// 可用变量：{title_cn} {title_en} {title_jp} {year} {season} {season:02} {ep} {ep:02} {ext} {group}
	TVTemplate    string // 如 "{title_cn} ({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}"
	MovieTemplate string // 如 "{title_cn} ({year})/{title_en}{ext}"

	// 是否硬链接（true=保留原文件，false=直接移动）
	UseHardLink bool
}

// AIConfig 大模型配置（全部可选，关掉 AI 系统仍能正常运行）
type AIConfig struct {
	Enabled bool

	// OpenAI 兼容接口（覆盖 OpenAI / DeepSeek / 通义 / 月之暗面 等）
	Endpoint string // API 地址，如 "https://api.openai.com/v1"
	APIKey   string
	Model    string // 如 "gpt-4o-mini" / "deepseek-chat"

	// Google Gemini（直接用 Gemini API）
	GeminiKey string

	// 本地 Ollama
	OllamaHost  string // 如 "http://127.0.0.1:11434"
	OllamaModel string // 如 "llama3.2"
}

// SchedulerConfig 定时任务配置
type SchedulerConfig struct {
	// RSS 轮询间隔，默认 30 分钟
	RSSInterval time.Duration

	// 检查缺集并触发补全的间隔，默认 24 小时
	SupplementInterval time.Duration

	// 整理完成文件的检查间隔，默认 2 分钟
	OrganizerInterval time.Duration
}

// ============================================================
// Load 加载配置（环境变量优先）
// ============================================================

// Load 从环境变量加载配置，未设置的用默认值兜底
// 这是目前最简单的实现，后续可以加 YAML 文件支持
func Load() *Config {
	cfg := defaults() // 先填默认值

	// 从环境变量覆盖（有设置才覆盖，没设置保留默认值）
	// 命名规则：AUTOANI_ 前缀 + 配置路径大写

	// Mikan
	if v := os.Getenv("MIKAN_RSS_URL"); v != "" {
		cfg.Mikan.PersonalRSSURL = v
	}
	if v := os.Getenv("MIKAN_DOMAIN"); v != "" {
		cfg.Mikan.Domain = v
	}
	if v := os.Getenv("MIKAN_PROXY_DOMAIN"); v != "" {
		cfg.Mikan.ProxyDomain = v
	}

	// qBittorrent（你现在在用的）
	if v := os.Getenv("QB_HOST"); v != "" {
		cfg.Downloaders.QBittorrent.Host = v
		cfg.Downloaders.QBittorrent.Enabled = true
	}
	if v := os.Getenv("QB_USER"); v != "" {
		cfg.Downloaders.QBittorrent.Username = v
	}
	if v := os.Getenv("QB_PASS"); v != "" {
		cfg.Downloaders.QBittorrent.Password = v
	}

	// TMDB
	if v := os.Getenv("TMDB_API_KEY"); v != "" {
		cfg.Metadata.TMDB.APIKey = v
		cfg.Metadata.TMDB.Enabled = true
	}

	// AI
	if v := os.Getenv("AI_ENDPOINT"); v != "" {
		cfg.AI.Endpoint = v
		cfg.AI.Enabled = true
	}
	if v := os.Getenv("AI_API_KEY"); v != "" {
		cfg.AI.APIKey = v
	}
	if v := os.Getenv("AI_MODEL"); v != "" {
		cfg.AI.Model = v
	}
	if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		cfg.AI.GeminiKey = v
		cfg.AI.Enabled = true
	}

	// 数据库路径
	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.Database.Path = v
	}

	// 服务器端口
	if v := os.Getenv("PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}

	// 整理目录（你可以在这里填自己的 fnOS 路径）
	if v := os.Getenv("TV_BASE_PATH"); v != "" {
		cfg.Organizer.TVBasePath = v
	}
	if v := os.Getenv("MOVIE_BASE_PATH"); v != "" {
		cfg.Organizer.MovieBasePath = v
	}

	return cfg
}

// defaults 返回所有配置的默认值
func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Database: DatabaseConfig{
			Path: "/data/autoani.db",
		},
		Mikan: MikanConfig{
			Domain: "mikanani.me",
		},
		Downloaders: DownloadersConfig{
			Default: "qbittorrent",
			QBittorrent: QBittorrentConfig{
				Host:     "http://localhost:8080",
				Category: "autoani",
			},
		},
		Metadata: MetadataConfig{
			Primary: "tmdb",
			TMDB: TMDBConfig{
				Language: "zh-CN",
			},
			BGMTV: BGMTVConfig{
				Enabled: true, // BGM.tv 不需要 key，默认开启
			},
		},
		Organizer: OrganizerConfig{
			TVBasePath:    "/TV/Media/番剧",
			MovieBasePath: "/TV/Media/剧场版",
			// 默认模板：用户可在 Web UI 设置页面修改
			TVTemplate:    "{title_cn} ({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
			MovieTemplate: "{title_cn} ({year})/{title_en}{ext}",
			UseHardLink:   false,
		},
		AI: AIConfig{
			Enabled: false, // 默认关闭，用户主动开启
			Model:   "gpt-4o-mini",
		},
		Scheduler: SchedulerConfig{
			RSSInterval:        30 * time.Minute,
			SupplementInterval: 24 * time.Hour,
			OrganizerInterval:  2 * time.Minute,
		},
	}
}
