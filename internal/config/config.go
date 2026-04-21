package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server      ServerConfig
	Database    DatabaseConfig
	Mikan       MikanConfig
	Downloaders DownloadersConfig
	Metadata    MetadataConfig
	Organizer   OrganizerConfig
	AI          AIConfig
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
	Enabled  bool
	APIKey   string
	Language string
}

type BGMTVConfig struct {
	Enabled   bool
	UserToken string
}

type OrganizerConfig struct {
	TVBasePath    string
	MovieBasePath string
	OVABasePath   string
	TVTemplate    string
	MovieTemplate string
	UseHardLink   bool
}

type AIConfig struct {
	Enabled     bool
	Endpoint    string
	APIKey      string
	Model       string
	GeminiKey   string
	OllamaHost  string
	OllamaModel string
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
	if v := os.Getenv("TMDB_API_KEY"); v != "" {
		cfg.Metadata.TMDB.APIKey = v
		cfg.Metadata.TMDB.Enabled = true
	}
	if v := os.Getenv("AI_ENDPOINT"); v != "" {
		cfg.AI.Endpoint = v
		cfg.AI.Enabled = true
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	return cfg
}

func defaults() *Config {
	return &Config{
		Server:   ServerConfig{Host: "0.0.0.0", Port: 8080},
		Database: DatabaseConfig{Path: "/data/ani-rss.db"},
		Mikan:    MikanConfig{Domain: "mikanani.me"},
		Downloaders: DownloadersConfig{
			Default: "qbittorrent",
			QBittorrent: QBittorrentConfig{
				Host: "http://localhost:8080", Category: "ani-rss",
			},
		},
		Metadata: MetadataConfig{
			Primary: "tmdb",
			TMDB:    TMDBConfig{Language: "zh-CN"},
			BGMTV:   BGMTVConfig{Enabled: true},
		},
		Organizer: OrganizerConfig{
			TVBasePath:    "/TV/Media/番剧",
			MovieBasePath: "/TV/Media/剧场版",
			TVTemplate:    "{title_cn} ({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
			MovieTemplate: "{title_cn} ({year})/{title_en}{ext}",
			UseHardLink:   false,
		},
		AI: AIConfig{Enabled: false, Model: "gpt-4o-mini"},
		Scheduler: SchedulerConfig{
			RSSInterval: 30 * time.Minute, SupplementInterval: 24 * time.Hour, OrganizerInterval: 2 * time.Minute,
		},
	}
}
