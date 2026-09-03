package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type Config struct {
	Server      ServerConfig
	Database    DatabaseConfig
	Mikan       MikanConfig
	Downloaders DownloadersConfig
	Sources     SourcesConfig
	Metadata    MetadataConfig
	Organizer   OrganizerConfig
	AI          AIConfig
	Notifier    NotifierConfig
	Scheduler   SchedulerConfig
}

type ServerConfig struct {
	Host           string
	Port           int
	LogPath        string
	AllowedOrigins []string // CORS 允许的 Origin 列表
}
type DatabaseConfig struct {
	Path string
}

type MikanConfig struct {
	PersonalRSSURL string
	Domain         string
	ProxyDomain    string
	MirrorDomains  []string // 镜像域名列表，GFW 环境下自动回退
	RSSMode        string   // "personal" 个人RSS（仅已有订阅）或 "classic" 经典RSS（全平台，自动建番剧）
}

type DownloadersConfig struct {
	Default      string
	QBittorrent  QBittorrentConfig
	Transmission TransmissionConfig
	Aria2        Aria2Config
}

type QBittorrentConfig struct {
	Enabled  bool
	Host     string
	Username string
	Password string
	Category string
}

type TransmissionConfig struct {
	Enabled  bool
	Host     string
	Username string
	Password string
}

type Aria2Config struct {
	Enabled bool
	Host    string
	Secret  string
}

type MetadataConfig struct {
	Primary string
	TMDB    TMDBConfig
	BGMTV   BGMTVConfig
}

type TMDBConfig struct {
	Enabled       bool
	APIKey        string
	Language      string
	MirrorDomains []string // TMDB API 镜像域名列表
}

type BGMTVConfig struct {
	Enabled       bool
	UserToken     string
	Username      string   // 个人同步用的用户名
	MirrorDomains []string // BGM 镜像域名 (bgm.tv, bangumi.tv, chii.in)
}

type OrganizerConfig struct {
	TVBasePath    string
	MovieBasePath string
	OVABasePath   string
	TVTemplate    string
	MovieTemplate string
	OtherTemplate string
	UseHardLink   bool
}

type SourcesConfig struct {
	Nyaa       SourceConfig
	ACGRIP     SourceConfig
	AnimeTosho SourceConfig
}

type SourceConfig struct {
	Enabled bool
	Domain  string
}

type AIConfig struct {
	Enabled            bool
	SmartSearchEnabled bool
	Protocol           string // openai / google / anthropic / ollama / auto
	Endpoint           string
	APIKey             string
	Model              string
	BackupModel        string
	GeminiKey          string
	ClaudeKey          string
	OllamaHost         string
	OllamaModel        string
}

type NotifierConfig struct {
	TelegramBotToken string
	TelegramChatID   string
	DiscordWebhook   string
	WecomWebhook     string
	FeishuWebhook    string
	DingTalkWebhook  string
	OneBotHost       string
	OneBotToken      string
	OneBotUserID     int64
	OneBotGroupID    int64
	SlackWebhook     string
	MatrixHomeserver string
	MatrixToken      string
	MatrixRoomID     string
	ServerChanKey    string
	BarkDeviceKey    string
	PushoverToken    string
	PushoverUser     string
	GotifyURL        string
	GotifyToken      string
	NtfyURL          string
	NtfyTopic        string
	EmailSMTPHost    string
	EmailSMTPPort    string
	EmailUsername    string
	EmailPassword    string
	EmailFrom        string
	EmailTo          string
	LINEChannelToken string
	LINEUserID       string
	WhatsAppPhoneID  string
	WhatsAppToken    string
	WhatsAppTo       string
}

type SchedulerConfig struct {
	RSSInterval         time.Duration
	SupplementInterval  time.Duration
	OrganizerInterval   time.Duration
	SyncBangumiInterval time.Duration // Bangumi 同步间隔
	// 种子自动清理配置
	SeedCleanupEnabled     bool          // 是否启用自动清理已完成种子
	SeedCleanupInterval    time.Duration // 清理检查间隔
	SeedCleanupMinSeedTime time.Duration // 最小做种时间（默认 48h）
	SeedCleanupMinRatio    float64       // 最小做种比率（默认 1.0）
	// 备份配置
	BackupPath      string // 备份存放路径，默认为 ./data/backups
	BackupCron      string // 定时任务 cron 表达式，如 "0 0 0 * * *" 每日凌晨
	BackupKeepCount int    // 保留备份份数，默认 7 份
}

func Load() *Config {
	cfg := defaults()
	if v := os.Getenv("ANI_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	} else if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("ANI_DB_PATH"); v != "" {
		cfg.Database.Path = v
	} else if v := os.Getenv("DB_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("MIKAN_RSS_URL"); v != "" {
		cfg.Mikan.PersonalRSSURL = v
	}
	if v := os.Getenv("MIKAN_DOMAIN"); v != "" {
		cfg.Mikan.Domain = v
	}
	if v := os.Getenv("MIKAN_PROXY_DOMAIN"); v != "" {
		cfg.Mikan.ProxyDomain = v
	}
	if v := os.Getenv("MIKAN_MIRROR_DOMAINS"); v != "" {
		cfg.Mikan.MirrorDomains = splitEnv(v)
	}
	if v := os.Getenv("MIKAN_RSS_MODE"); v != "" {
		cfg.Mikan.RSSMode = v
	}
	// 校验：非 classic 且非 personal 时回退到 classic
	if cfg.Mikan.RSSMode != core.RSSModeClassic && cfg.Mikan.RSSMode != core.RSSModePersonal {
		log.Printf("⚠️ 无效的 MIKAN_RSS_MODE=%q，回退为 classic", cfg.Mikan.RSSMode)
		cfg.Mikan.RSSMode = core.RSSModeClassic
	}
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
	if v := os.Getenv("QB_CATEGORY"); v != "" {
		cfg.Downloaders.QBittorrent.Category = v
	}
	if v := os.Getenv("TR_HOST"); v != "" {
		cfg.Downloaders.Transmission.Host = v
		cfg.Downloaders.Transmission.Enabled = true
	}
	if v := os.Getenv("TR_USER"); v != "" {
		cfg.Downloaders.Transmission.Username = v
	}
	if v := os.Getenv("TR_PASS"); v != "" {
		cfg.Downloaders.Transmission.Password = v
	}
	if v := os.Getenv("ARIA2_HOST"); v != "" {
		cfg.Downloaders.Aria2.Host = v
		cfg.Downloaders.Aria2.Enabled = true
	}
	if v := os.Getenv("ARIA2_SECRET"); v != "" {
		cfg.Downloaders.Aria2.Secret = v
	}
	if v := os.Getenv("DOWNLOADER_DEFAULT"); v != "" {
		cfg.Downloaders.Default = v
	}
	if v := os.Getenv("METADATA_PRIMARY"); v != "" {
		cfg.Metadata.Primary = v
	}
	if v := os.Getenv("TMDB_API_KEY"); v != "" {
		cfg.Metadata.TMDB.APIKey = v
		cfg.Metadata.TMDB.Enabled = true
	}
	if v := os.Getenv("TMDB_LANGUAGE"); v != "" {
		cfg.Metadata.TMDB.Language = v
	}
	if v := os.Getenv("TMDB_MIRROR_DOMAINS"); v != "" {
		cfg.Metadata.TMDB.MirrorDomains = splitEnv(v)
	}
	if v := os.Getenv("BGMTV_USER_TOKEN"); v != "" {
		cfg.Metadata.BGMTV.UserToken = v
		cfg.Metadata.BGMTV.Enabled = true
	}
	if v := os.Getenv("BGMTV_USERNAME"); v != "" {
		cfg.Metadata.BGMTV.Username = v
	}
	if v := os.Getenv("BGMTV_MIRROR_DOMAINS"); v != "" {
		cfg.Metadata.BGMTV.MirrorDomains = splitEnv(v)
	}
	if v := os.Getenv("BGMTV_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Scheduler.SyncBangumiInterval = d
		}
	}
	if v := os.Getenv("TV_BASE_PATH"); v != "" {
		cfg.Organizer.TVBasePath = v
	}
	if v := os.Getenv("MOVIE_BASE_PATH"); v != "" {
		cfg.Organizer.MovieBasePath = v
	}
	if v := os.Getenv("OVA_BASE_PATH"); v != "" {
		cfg.Organizer.OVABasePath = v
	}
	if v := os.Getenv("TV_TEMPLATE"); v != "" {
		cfg.Organizer.TVTemplate = v
	}
	if v := os.Getenv("MOVIE_TEMPLATE"); v != "" {
		cfg.Organizer.MovieTemplate = v
	}
	if v := os.Getenv("USE_HARDLINK"); v != "" {
		cfg.Organizer.UseHardLink = v == "true"
	}
	if v := os.Getenv("AI_ENABLED"); v != "" {
		cfg.AI.Enabled = v == "true"
	}
	if v := os.Getenv("AI_SMART_SEARCH"); v != "" {
		cfg.AI.SmartSearchEnabled = v == "true"
	}
	if v := os.Getenv("AI_PROTOCOL"); v != "" {
		cfg.AI.Protocol = v
	}
	if v := os.Getenv("AI_ENDPOINT"); v != "" {
		cfg.AI.Endpoint = v
	}
	if v := os.Getenv("AI_API_KEY"); v != "" {
		cfg.AI.APIKey = v
	}
	if v := os.Getenv("AI_MODEL"); v != "" {
		cfg.AI.Model = v
	}
	if v := os.Getenv("AI_BACKUP_MODEL"); v != "" {
		cfg.AI.BackupModel = v
	}
	if v := os.Getenv("TG_BOT_TOKEN"); v != "" {
		cfg.Notifier.TelegramBotToken = v
	}
	if v := os.Getenv("TG_CHAT_ID"); v != "" {
		cfg.Notifier.TelegramChatID = v
	}
	if v := os.Getenv("DISCORD_WEBHOOK"); v != "" {
		cfg.Notifier.DiscordWebhook = v
	}
	if v := os.Getenv("WECOM_WEBHOOK"); v != "" {
		cfg.Notifier.WecomWebhook = v
	}
	if v := os.Getenv("FEISHU_WEBHOOK"); v != "" {
		cfg.Notifier.FeishuWebhook = v
	}
	if v := os.Getenv("DINGTALK_WEBHOOK"); v != "" {
		cfg.Notifier.DingTalkWebhook = v
	}
	if v := os.Getenv("ONEBOT_HOST"); v != "" {
		cfg.Notifier.OneBotHost = v
	}
	if v := os.Getenv("ONEBOT_TOKEN"); v != "" {
		cfg.Notifier.OneBotToken = v
	}
	if v := os.Getenv("ONEBOT_USER_ID"); v != "" {
		id, _ := strconv.ParseInt(v, 10, 64)
		cfg.Notifier.OneBotUserID = id
	}
	if v := os.Getenv("ONEBOT_GROUP_ID"); v != "" {
		id, _ := strconv.ParseInt(v, 10, 64)
		cfg.Notifier.OneBotGroupID = id
	}
	if v := os.Getenv("SLACK_WEBHOOK"); v != "" {
		cfg.Notifier.SlackWebhook = v
	}
	if v := os.Getenv("MATRIX_HOMESERVER"); v != "" {
		cfg.Notifier.MatrixHomeserver = v
	}
	if v := os.Getenv("MATRIX_TOKEN"); v != "" {
		cfg.Notifier.MatrixToken = v
	}
	if v := os.Getenv("MATRIX_ROOM_ID"); v != "" {
		cfg.Notifier.MatrixRoomID = v
	}
	if v := os.Getenv("SERVERCHAN_KEY"); v != "" {
		cfg.Notifier.ServerChanKey = v
	}
	if v := os.Getenv("BARK_DEVICE_KEY"); v != "" {
		cfg.Notifier.BarkDeviceKey = v
	}
	if v := os.Getenv("PUSHOVER_TOKEN"); v != "" {
		cfg.Notifier.PushoverToken = v
	}
	if v := os.Getenv("PUSHOVER_USER"); v != "" {
		cfg.Notifier.PushoverUser = v
	}
	if v := os.Getenv("GOTIFY_URL"); v != "" {
		cfg.Notifier.GotifyURL = v
	}
	if v := os.Getenv("GOTIFY_TOKEN"); v != "" {
		cfg.Notifier.GotifyToken = v
	}
	if v := os.Getenv("NTFY_URL"); v != "" {
		cfg.Notifier.NtfyURL = v
	}
	if v := os.Getenv("NTFY_TOPIC"); v != "" {
		cfg.Notifier.NtfyTopic = v
	}
	if v := os.Getenv("SMTP_HOST"); v != "" {
		cfg.Notifier.EmailSMTPHost = v
	}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		cfg.Notifier.EmailSMTPPort = v
	}
	if v := os.Getenv("SMTP_USER"); v != "" {
		cfg.Notifier.EmailUsername = v
	}
	if v := os.Getenv("SMTP_PASS"); v != "" {
		cfg.Notifier.EmailPassword = v
	}
	if v := os.Getenv("EMAIL_FROM"); v != "" {
		cfg.Notifier.EmailFrom = v
	}
	if v := os.Getenv("EMAIL_TO"); v != "" {
		cfg.Notifier.EmailTo = v
	}
	if v := os.Getenv("LINE_TOKEN"); v != "" {
		cfg.Notifier.LINEChannelToken = v
	}
	if v := os.Getenv("LINE_USER_ID"); v != "" {
		cfg.Notifier.LINEUserID = v
	}
	if v := os.Getenv("WHATSAPP_PHONE_ID"); v != "" {
		cfg.Notifier.WhatsAppPhoneID = v
	}
	if v := os.Getenv("WHATSAPP_TOKEN"); v != "" {
		cfg.Notifier.WhatsAppToken = v
	}
	if v := os.Getenv("WHATSAPP_TO"); v != "" {
		cfg.Notifier.WhatsAppTo = v
	}
	if v := os.Getenv("RSS_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Scheduler.RSSInterval = d
		}
	}
	if v := os.Getenv("SUPPLEMENT_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Scheduler.SupplementInterval = d
		}
	}
	if v := os.Getenv("ORGANIZER_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Scheduler.OrganizerInterval = d
		}
	}
	if v := os.Getenv("SEED_CLEANUP_ENABLED"); v != "" {
		cfg.Scheduler.SeedCleanupEnabled = v == "true"
	}
	if v := os.Getenv("SEED_CLEANUP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Scheduler.SeedCleanupInterval = d
		}
	}
	if v := os.Getenv("SEED_CLEANUP_MIN_TIME"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Scheduler.SeedCleanupMinSeedTime = d
		}
	}
	if v := os.Getenv("SEED_CLEANUP_MIN_RATIO"); v != "" {
		if r, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Scheduler.SeedCleanupMinRatio = r
		}
	}
	if v := os.Getenv("BACKUP_PATH"); v != "" {
		cfg.Scheduler.BackupPath = v
	}
	if v := os.Getenv("BACKUP_CRON"); v != "" {
		cfg.Scheduler.BackupCron = v
	}
	if v := os.Getenv("BACKUP_KEEP_COUNT"); v != "" {
		if count, err := strconv.Atoi(v); err == nil {
			cfg.Scheduler.BackupKeepCount = count
		}
	}
	if v := os.Getenv("NYAA_DOMAIN"); v != "" {
		cfg.Sources.Nyaa.Domain = v
		cfg.Sources.Nyaa.Enabled = true
	}
	if v := os.Getenv("ACGRIP_DOMAIN"); v != "" {
		cfg.Sources.ACGRIP.Domain = v
		cfg.Sources.ACGRIP.Enabled = true
	}
	if v := os.Getenv("ANIMETOSHO_DOMAIN"); v != "" {
		cfg.Sources.AnimeTosho.Domain = v
		cfg.Sources.AnimeTosho.Enabled = true
	}
	return cfg
}

func splitEnv(v string) []string {
	// 支持逗号和空格分隔
	v = strings.ReplaceAll(v, ",", " ")
	parts := strings.Fields(v)
	res := make([]string, 0, len(parts))
	res = append(res, parts...)
	return res
}

func defaults() *Config {
	return &Config{
		Server:   ServerConfig{Host: "0.0.0.0", Port: 20001, LogPath: "./data/ani-go.log", AllowedOrigins: []string{"http://localhost:3000", "http://localhost:5173", "http://localhost:20001"}},
		Database: DatabaseConfig{Path: "ani-go.db"},
		Mikan: MikanConfig{
			Domain: "mikanime.tv", MirrorDomains: []string{"mikanime.tv", "mikanani.kas.pub", "mikanani.me"},
			RSSMode: core.RSSModePersonal,
		},
		Downloaders: DownloadersConfig{
			Default: "qbittorrent",
			QBittorrent: QBittorrentConfig{
				Host: "http://localhost:8081", Category: "ani-go",
			},
		},
		Metadata: MetadataConfig{
			Primary: "tmdb",
			TMDB:    TMDBConfig{Language: "zh-CN"},
			BGMTV: BGMTVConfig{
				Enabled: true, MirrorDomains: []string{"api.bgm.tv", "api.bangumi.tv", "api.chii.in"},
			},
		},
		Organizer: OrganizerConfig{
			TVBasePath:    "./TV/番剧",
			MovieBasePath: "./TV/剧场版",
			OVABasePath:   "./TV/OVA",
			TVTemplate:    "{title_cn}{year}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
			MovieTemplate: "{title_cn}{year}/{title_en}{ext}",
			UseHardLink:   false,
		},
		Sources: SourcesConfig{
			Nyaa:       SourceConfig{Domain: "nyaa.si"},
			ACGRIP:     SourceConfig{Domain: "acg.rip"},
			AnimeTosho: SourceConfig{Domain: "feed.animetosho.org"},
		},
		AI:       AIConfig{Enabled: false, Model: "gpt-4o-mini", SmartSearchEnabled: true},
		Notifier: NotifierConfig{},
		Scheduler: SchedulerConfig{
			RSSInterval: 30 * time.Minute, SupplementInterval: 24 * time.Hour, OrganizerInterval: 2 * time.Minute,
			SyncBangumiInterval:    6 * time.Hour,
			SeedCleanupEnabled:     true, SeedCleanupInterval: 1 * time.Hour,
			SeedCleanupMinSeedTime: 48 * time.Hour, SeedCleanupMinRatio: 1.0,
			BackupPath:      "./data/backups",
			BackupCron:      "0 0 0 * * *",
			BackupKeepCount: 7,
		},
	}
}

// MergeFromSettings 从数据库设置表读取配置，填充 env var 未设置的字段
// 优先级：环境变量 > 数据库设置 > 默认值
func (c *Config) MergeFromSettings(getter func(key string) (string, bool)) {
	if v, ok := getter("MIKAN_RSS_URL"); ok && c.Mikan.PersonalRSSURL == "" {
		c.Mikan.PersonalRSSURL = v
	}
	if v, ok := getter("MIKAN_DOMAIN"); ok && c.Mikan.Domain == "" {
		c.Mikan.Domain = v
	}
	if v, ok := getter("MIKAN_RSS_MODE"); ok && c.Mikan.RSSMode == "" {
		c.Mikan.RSSMode = v
	}
	if v, ok := getter("QB_HOST"); ok && c.Downloaders.QBittorrent.Host == "" {
		c.Downloaders.QBittorrent.Host = v
		c.Downloaders.QBittorrent.Enabled = true
	}
	if v, ok := getter("QB_USER"); ok && c.Downloaders.QBittorrent.Username == "" {
		c.Downloaders.QBittorrent.Username = v
	}
	if v, ok := getter("QB_PASS"); ok && c.Downloaders.QBittorrent.Password == "" {
		c.Downloaders.QBittorrent.Password = v
	}
	if v, ok := getter("QB_CATEGORY"); ok && c.Downloaders.QBittorrent.Category == "" {
		c.Downloaders.QBittorrent.Category = v
	}
	if v, ok := getter("METADATA_PRIMARY"); ok && c.Metadata.Primary == "" {
		c.Metadata.Primary = v
	}
	if v, ok := getter("TMDB_API_KEY"); ok && c.Metadata.TMDB.APIKey == "" {
		c.Metadata.TMDB.APIKey = v
		c.Metadata.TMDB.Enabled = true
	}
	if v, ok := getter("BGMTV_USER_TOKEN"); ok && c.Metadata.BGMTV.UserToken == "" {
		c.Metadata.BGMTV.UserToken = v
		c.Metadata.BGMTV.Enabled = true
	}
	if v, ok := getter("BGMTV_USERNAME"); ok && c.Metadata.BGMTV.Username == "" {
		c.Metadata.BGMTV.Username = v
	}
	if v, ok := getter("DOWNLOADER_DEFAULT"); ok && c.Downloaders.Default == "qbittorrent" {
		c.Downloaders.Default = v
	}
	if v, ok := getter("BGMTV_SYNC_INTERVAL"); ok && c.Scheduler.SyncBangumiInterval == 0 {
		if d, err := time.ParseDuration(v); err == nil {
			c.Scheduler.SyncBangumiInterval = d
		}
	}
	if v, ok := getter("TV_BASE_PATH"); ok && c.Organizer.TVBasePath == "" {
		c.Organizer.TVBasePath = v
	}
	if v, ok := getter("MOVIE_BASE_PATH"); ok && c.Organizer.MovieBasePath == "" {
		c.Organizer.MovieBasePath = v
	}
	if v, ok := getter("OVA_BASE_PATH"); ok && c.Organizer.OVABasePath == "" {
		c.Organizer.OVABasePath = v
	}
	if v, ok := getter("AI_ENABLED"); ok {
		c.AI.Enabled = v == "true"
	}
	if v, ok := getter("AI_API_KEY"); ok && c.AI.APIKey == "" {
		c.AI.APIKey = v
	}
	if v, ok := getter("AI_MODEL"); ok && c.AI.Model == "" {
		c.AI.Model = v
	}
	if v, ok := getter("TV_TEMPLATE"); ok && (c.Organizer.TVTemplate == "" || c.Organizer.TVTemplate == "old-tv") {
		c.Organizer.TVTemplate = v
	}
	if v, ok := getter("MOVIE_TEMPLATE"); ok && c.Organizer.MovieTemplate == "" {
		c.Organizer.MovieTemplate = v
	}
	if v, ok := getter("OTHER_TEMPLATE"); ok && c.Organizer.OtherTemplate == "" {
		c.Organizer.OtherTemplate = v
	}
}
