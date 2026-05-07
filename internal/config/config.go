package config

import (
	"os"
	"strconv"
	"strings"
	"time"
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
	Host string
	Port int
}

type DatabaseConfig struct {
	Path string
}

type MikanConfig struct {
	PersonalRSSURL string
	Domain         string
	ProxyDomain    string
	MirrorDomains  []string // 镜像域名列表，GFW 环境下自动回退
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
	MirrorDomains []string // BGM 镜像域名 (bgm.tv, bangumi.tv, chii.in)
}

type OrganizerConfig struct {
	TVBasePath    string
	MovieBasePath string
	OVABasePath   string
	TVTemplate    string
	MovieTemplate string
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
	Enabled     bool
	Protocol    string // openai / google / anthropic / ollama / auto
	Endpoint    string
	APIKey      string
	Model       string
	GeminiKey   string
	ClaudeKey   string
	OllamaHost  string
	OllamaModel string
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
	RSSInterval        time.Duration
	SupplementInterval time.Duration
	OrganizerInterval  time.Duration
}

func Load() *Config {
	cfg := defaults()
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
	if v := os.Getenv("DEFAULT_DOWNLOADER"); v != "" {
		cfg.Downloaders.Default = v
	}
	if v := os.Getenv("ARIA2_HOST"); v != "" {
		cfg.Downloaders.Aria2.Host = v
		cfg.Downloaders.Aria2.Enabled = true
	}
	if v := os.Getenv("ARIA2_SECRET"); v != "" {
		cfg.Downloaders.Aria2.Secret = v
	}
	if v := os.Getenv("TMDB_API_KEY"); v != "" {
		cfg.Metadata.TMDB.APIKey = v
		cfg.Metadata.TMDB.Enabled = true
	}
	if v := os.Getenv("TMDB_MIRROR_DOMAINS"); v != "" {
		cfg.Metadata.TMDB.MirrorDomains = splitEnv(v)
	}
	if v := os.Getenv("BGMTV_USER_TOKEN"); v != "" {
		cfg.Metadata.BGMTV.UserToken = v
		cfg.Metadata.BGMTV.Enabled = true
	}
	if v := os.Getenv("BGMTV_MIRROR_DOMAINS"); v != "" {
		cfg.Metadata.BGMTV.MirrorDomains = splitEnv(v)
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
	if v := os.Getenv("AI_PROTOCOL"); v != "" {
		cfg.AI.Protocol = v
	}
	if v := os.Getenv("AI_ENDPOINT"); v != "" {
		cfg.AI.Endpoint = v
		cfg.AI.Enabled = true
	}
	if v := os.Getenv("AI_API_KEY"); v != "" {
		cfg.AI.APIKey = v
		cfg.AI.Enabled = true
	}
	if v := os.Getenv("AI_MODEL"); v != "" {
		cfg.AI.Model = v
	}
	if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		cfg.AI.GeminiKey = v
		cfg.AI.Enabled = true
	}
	if v := os.Getenv("CLAUDE_API_KEY"); v != "" {
		cfg.AI.ClaudeKey = v
		cfg.AI.Enabled = true
	}
	if v := os.Getenv("OLLAMA_HOST"); v != "" {
		cfg.AI.OllamaHost = v
		cfg.AI.Enabled = true
	}
	if v := os.Getenv("OLLAMA_MODEL"); v != "" {
		cfg.AI.OllamaModel = v
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
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
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Notifier.TelegramBotToken = v
	}
	if v := os.Getenv("TELEGRAM_CHAT_ID"); v != "" {
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
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Notifier.OneBotUserID = id
		}
	}
	if v := os.Getenv("ONEBOT_GROUP_ID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Notifier.OneBotGroupID = id
		}
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
	if v := os.Getenv("EMAIL_SMTP_HOST"); v != "" {
		cfg.Notifier.EmailSMTPHost = v
	}
	if v := os.Getenv("EMAIL_SMTP_PORT"); v != "" {
		cfg.Notifier.EmailSMTPPort = v
	}
	if v := os.Getenv("EMAIL_USERNAME"); v != "" {
		cfg.Notifier.EmailUsername = v
	}
	if v := os.Getenv("EMAIL_PASSWORD"); v != "" {
		cfg.Notifier.EmailPassword = v
	}
	if v := os.Getenv("EMAIL_FROM"); v != "" {
		cfg.Notifier.EmailFrom = v
	}
	if v := os.Getenv("EMAIL_TO"); v != "" {
		cfg.Notifier.EmailTo = v
	}
	if v := os.Getenv("LINE_CHANNEL_TOKEN"); v != "" {
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
	return cfg
}

func defaults() *Config {
	return &Config{
		Server:   ServerConfig{Host: "0.0.0.0", Port: 20001},
		Database: DatabaseConfig{Path: "ani-go.db"},
		Mikan: MikanConfig{
			Domain: "mikanime.tv", MirrorDomains: []string{"mikanime.tv", "mikanani.kas.pub", "mikanani.me"},
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
			TVTemplate:    "{title_cn} ({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
			MovieTemplate: "{title_cn} ({year})/{title_en}{ext}",
			UseHardLink:   false,
		},
		Sources: SourcesConfig{
			Nyaa:       SourceConfig{Domain: "nyaa.si"},
			ACGRIP:     SourceConfig{Domain: "acg.rip"},
			AnimeTosho: SourceConfig{Domain: "feed.animetosho.org"},
		},
		AI: AIConfig{Enabled: false, Model: "gpt-4o-mini"},
		Notifier: NotifierConfig{},
		Scheduler: SchedulerConfig{
			RSSInterval: 30 * time.Minute, SupplementInterval: 24 * time.Hour, OrganizerInterval: 2 * time.Minute,
		},
	}
}

// MergeFromSettings 从数据库设置表读取配置，填充 env var 未设置的字段
// 优先级：环境变量 > 数据库设置 > 默认值
func (c *Config) MergeFromSettings(getter func(key string) (string, bool)) {
	if getter == nil {
		return
	}
	if v, ok := getter("MIKAN_RSS_URL"); ok && c.Mikan.PersonalRSSURL == "" {
		c.Mikan.PersonalRSSURL = v
	}
	if v, ok := getter("MIKAN_DOMAIN"); ok && c.Mikan.Domain == "mikanani.me" {
		c.Mikan.Domain = v
	}
	if v, ok := getter("MIKAN_PROXY_DOMAIN"); ok && c.Mikan.ProxyDomain == "" {
		c.Mikan.ProxyDomain = v
	}
	if v, ok := getter("MIKAN_MIRROR_DOMAINS"); ok {
		c.Mikan.MirrorDomains = splitEnv(v)
	}
	// 下载器
	if v, ok := getter("DEFAULT_DOWNLOADER"); ok && c.Downloaders.Default == "qbittorrent" {
		c.Downloaders.Default = v
	}
	if v, ok := getter("QB_HOST"); ok && c.Downloaders.QBittorrent.Host == "http://localhost:8081" {
		c.Downloaders.QBittorrent.Host = v
		c.Downloaders.QBittorrent.Enabled = true
	}
	if v, ok := getter("QB_USER"); ok && c.Downloaders.QBittorrent.Username == "" {
		c.Downloaders.QBittorrent.Username = v
	}
	if v, ok := getter("QB_PASS"); ok && c.Downloaders.QBittorrent.Password == "" {
		c.Downloaders.QBittorrent.Password = v
	}
	if v, ok := getter("TR_HOST"); ok && c.Downloaders.Transmission.Host == "" {
		c.Downloaders.Transmission.Host = v
		c.Downloaders.Transmission.Enabled = true
	}
	if v, ok := getter("TR_USER"); ok && c.Downloaders.Transmission.Username == "" {
		c.Downloaders.Transmission.Username = v
	}
	if v, ok := getter("TR_PASS"); ok && c.Downloaders.Transmission.Password == "" {
		c.Downloaders.Transmission.Password = v
	}
	if v, ok := getter("ARIA2_HOST"); ok && c.Downloaders.Aria2.Host == "" {
		c.Downloaders.Aria2.Host = v
		c.Downloaders.Aria2.Enabled = true
	}
	if v, ok := getter("ARIA2_SECRET"); ok && c.Downloaders.Aria2.Secret == "" {
		c.Downloaders.Aria2.Secret = v
	}
	// 目录
	if v, ok := getter("TV_BASE_PATH"); ok && c.Organizer.TVBasePath == "./TV/番剧" {
		c.Organizer.TVBasePath = v
	}
	if v, ok := getter("MOVIE_BASE_PATH"); ok && c.Organizer.MovieBasePath == "./TV/剧场版" {
		c.Organizer.MovieBasePath = v
	}
	if v, ok := getter("OVA_BASE_PATH"); ok && c.Organizer.OVABasePath == "./TV/OVA" {
		c.Organizer.OVABasePath = v
	}
	// 元数据
	if v, ok := getter("TMDB_API_KEY"); ok && c.Metadata.TMDB.APIKey == "" {
		c.Metadata.TMDB.APIKey = v
		c.Metadata.TMDB.Enabled = true
	}
	if v, ok := getter("BGMTV_USER_TOKEN"); ok && c.Metadata.BGMTV.UserToken == "" {
		c.Metadata.BGMTV.UserToken = v
		c.Metadata.BGMTV.Enabled = true
	}
	// AI
	if v, ok := getter("AI_PROTOCOL"); ok && c.AI.Protocol == "" {
		c.AI.Protocol = v
	}
	if v, ok := getter("AI_ENDPOINT"); ok && c.AI.Endpoint == "" {
		c.AI.Endpoint = v
		c.AI.Enabled = true
	}
	if v, ok := getter("AI_API_KEY"); ok && c.AI.APIKey == "" {
		c.AI.APIKey = v
		c.AI.Enabled = true
	}
	if v, ok := getter("AI_MODEL"); ok && c.AI.Model == "gpt-4o-mini" {
		c.AI.Model = v
	}
	if v, ok := getter("GEMINI_API_KEY"); ok && c.AI.GeminiKey == "" {
		c.AI.GeminiKey = v
		c.AI.Enabled = true
	}
	if v, ok := getter("CLAUDE_API_KEY"); ok && c.AI.ClaudeKey == "" {
		c.AI.ClaudeKey = v
		c.AI.Enabled = true
	}
	if v, ok := getter("OLLAMA_HOST"); ok && c.AI.OllamaHost == "" {
		c.AI.OllamaHost = v
		c.AI.Enabled = true
	}
	if v, ok := getter("OLLAMA_MODEL"); ok && c.AI.OllamaModel == "" {
		c.AI.OllamaModel = v
	}
	// 通知渠道
	if v, ok := getter("TELEGRAM_BOT_TOKEN"); ok && c.Notifier.TelegramBotToken == "" {
		c.Notifier.TelegramBotToken = v
	}
	if v, ok := getter("TELEGRAM_CHAT_ID"); ok && c.Notifier.TelegramChatID == "" {
		c.Notifier.TelegramChatID = v
	}
	if v, ok := getter("DISCORD_WEBHOOK"); ok && c.Notifier.DiscordWebhook == "" {
		c.Notifier.DiscordWebhook = v
	}
	if v, ok := getter("WECOM_WEBHOOK"); ok && c.Notifier.WecomWebhook == "" {
		c.Notifier.WecomWebhook = v
	}
	if v, ok := getter("FEISHU_WEBHOOK"); ok && c.Notifier.FeishuWebhook == "" {
		c.Notifier.FeishuWebhook = v
	}
	if v, ok := getter("DINGTALK_WEBHOOK"); ok && c.Notifier.DingTalkWebhook == "" {
		c.Notifier.DingTalkWebhook = v
	}
	if v, ok := getter("SLACK_WEBHOOK"); ok && c.Notifier.SlackWebhook == "" {
		c.Notifier.SlackWebhook = v
	}
	if v, ok := getter("LINE_CHANNEL_TOKEN"); ok && c.Notifier.LINEChannelToken == "" {
		c.Notifier.LINEChannelToken = v
	}
	if v, ok := getter("LINE_USER_ID"); ok && c.Notifier.LINEUserID == "" {
		c.Notifier.LINEUserID = v
	}
	if v, ok := getter("WHATSAPP_PHONE_ID"); ok && c.Notifier.WhatsAppPhoneID == "" {
		c.Notifier.WhatsAppPhoneID = v
	}
	if v, ok := getter("WHATSAPP_TOKEN"); ok && c.Notifier.WhatsAppToken == "" {
		c.Notifier.WhatsAppToken = v
	}
	if v, ok := getter("WHATSAPP_TO"); ok && c.Notifier.WhatsAppTo == "" {
		c.Notifier.WhatsAppTo = v
	}
	if v, ok := getter("EMAIL_SMTP_HOST"); ok && c.Notifier.EmailSMTPHost == "" {
		c.Notifier.EmailSMTPHost = v
	}
	if v, ok := getter("EMAIL_SMTP_PORT"); ok && c.Notifier.EmailSMTPPort == "" {
		c.Notifier.EmailSMTPPort = v
	}
	if v, ok := getter("EMAIL_USERNAME"); ok && c.Notifier.EmailUsername == "" {
		c.Notifier.EmailUsername = v
	}
	if v, ok := getter("EMAIL_PASSWORD"); ok && c.Notifier.EmailPassword == "" {
		c.Notifier.EmailPassword = v
	}
	if v, ok := getter("EMAIL_FROM"); ok && c.Notifier.EmailFrom == "" {
		c.Notifier.EmailFrom = v
	}
	if v, ok := getter("EMAIL_TO"); ok && c.Notifier.EmailTo == "" {
		c.Notifier.EmailTo = v
	}
	// 端口
	if v, ok := getter("PORT"); ok {
		if port, err := strconv.Atoi(v); err == nil && c.Server.Port == 20001 {
			c.Server.Port = port
		}
	}
	// 数据库路径
	if v, ok := getter("DB_PATH"); ok && c.Database.Path == "ani-go.db" {
		c.Database.Path = v
	}
}

// splitEnv 将逗号或空格分隔的环境变量值拆分为字符串切片
func splitEnv(v string) []string {
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == ' '
	})
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
