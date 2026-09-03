package core

import (
	"context"
	"strings"
	"time"
)

// ============================================================
// 万能上下文接口
// ============================================================

// ServiceType 服务类型标识
type ServiceType string

const (
	ServiceDB             ServiceType = "database"
	ServiceConfig         ServiceType = "config"
	ServiceDownloader     ServiceType = "downloader"
	ServiceSource         ServiceType = "source"
	ServiceOrganizer      ServiceType = "organizer"
	ServiceMetadata       ServiceType = "metadata"
	ServiceAI             ServiceType = "ai"
	ServiceEventBus       ServiceType = "eventbus"
	ServiceScheduler      ServiceType = "scheduler"
	ServiceNotifier       ServiceType = "notifier"
	ServicePluginManager  ServiceType = "plugin_manager"
	ServiceBackupManager  ServiceType = "backup_manager"
	ServiceTaskParser     ServiceType = "task_parser"
	ServiceSearch         ServiceType = "search"
)

// Context 万能上下文接口
// 所有组件通过 Context 获取依赖，实现完全解耦
type Context interface {
	context.Context

	// GetService 根据类型获取服务实例
	GetService(t ServiceType) (interface{}, bool)

	// SetService 注册服务实例
	SetService(t ServiceType, svc interface{})

	// MustGetService 获取服务，不存在则 panic（仅用于启动阶段）
	MustGetService(t ServiceType) interface{}

	// GetDB 获取数据库实例
	GetDB() interface{}

	// GetConfig 获取配置实例
	GetConfig() interface{}

	// GetEventBus 获取事件总线
	GetEventBus() EventBus

	// GetPluginManager 获取插件管理器
	GetPluginManager() interface{}

	// GetOrganizer 获取整理器
	GetOrganizer() Organizer

	// GetDownloader 获取下载器
	GetDownloader() Downloader

	// GetSource 获取资源源
	GetSource() Source

	// GetMetadataProvider 获取元数据提供者
	GetMetadataProvider() MetadataProvider

	// GetAIClassifier 获取 AI 分类器
	GetAIClassifier() AIClassifier

	// GetScheduler 获取调度器
	GetScheduler() interface{}

	// GetNotifier 获取通知管理器
	GetNotifier() interface{}

	// GetBackupManager 获取备份管理器
	GetBackupManager() interface{}

	// GetTaskParser 获取任务解析器
	GetTaskParser() TaskParser

	// GetSearch 获取搜索引擎
	GetSearch() interface{}
}

// ============================================================
// AI 分类器接口
// ============================================================

// AIClassifier AI 分类器接口（用于智能搜索、任务解析等）
type AIClassifier interface {
	// Classify 对文本进行分类/判断
	Classify(ctx context.Context, prompt string) (string, error)

	// IsAvailable 检查是否可用
	IsAvailable(ctx context.Context) bool

	// Name 返回提供者名称
	Name() string
}

// TorrentItem 统一种子条目
// json 标签与前端 Search.vue TorrentItem 接口保持同步
type TorrentItem struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	MagnetURL   string    `json:"magnet"`
	InfoHash    string    `json:"info_hash"`
	Size        int64     `json:"size"`
	PublishedAt time.Time `json:"pub_date"`
	SourceName  string    `json:"source"`
	BangumiID   string    `json:"bangumi_id"`
	EpisodeURL  string    `json:"episode_url,omitempty"`
	CoverURL    string    `json:"cover_url,omitempty"`
	AiredTime   string    `json:"aired_time,omitempty"`
	AiredDate   string    `json:"aired_date,omitempty"`
	GroupName   string    `json:"group_name"`
	Resolution  string    `json:"resolution,omitempty"`
	// YucWiki 元数据增强
	TotalEpisodes int  `json:"total_episodes,omitempty"`
	IsFinished    bool `json:"is_finished,omitempty"`
}

type Anime struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	TitleCN     string `json:"title_cn"`
	TitleEN     string `json:"title_en"`
	TitleJP     string `json:"title_jp"`
	Year        int    `json:"year"`
	Season      int    `json:"season"`
	TotalEps    int    `json:"total_eps"`
	Type        string `json:"type"`
	Description string `json:"description"`
	CoverURL    string `json:"cover_url"`
	SeriesID    string `json:"series_id"`
	TMDBID      string `json:"tmdb_id,omitempty"`
	IMDBID      string `json:"imdb_id,omitempty"`
}

type Episode struct {
	AnimeID string
	Season  int
	Number  float32
	Title   string
	AiredAt time.Time
}

type Subscription struct {
	ID           uint
	Title        string
	BangumiID    string
	AnimeID      string
	SourceName   string
	SubgroupPref []string
	DownloadPath string
	Filter       Filter
	Enabled      bool
	Completed    bool
}

type Filter struct {
	IncludeKeywords []string
	ExcludeKeywords []string
	PreferSubgroup  string
	Resolution      string
	AllowedSubgroups []string
}

type DownloadTask struct {
	Hash         string
	Name         string
	SavePath     string
	ContentPath  string
	Status       string
	Progress     float32
	SpeedDown    int64
	Size         int64
	Done         int64
}

type Event struct {
	Type    string
	Payload map[string]interface{}
	Time    time.Time
}

type Source interface {
	Name() string
	FetchRSS(ctx context.Context, url string) ([]TorrentItem, error)
	SearchAnime(ctx context.Context, title string) ([]TorrentItem, error)
	FetchHistory(ctx context.Context, bangumiID string, filter Filter) ([]TorrentItem, error)
	IsAvailable(ctx context.Context) bool
}

type Downloader interface {
	Name() string
	Add(ctx context.Context, item TorrentItem, savePath string) error
	List(ctx context.Context) ([]DownloadTask, error)
	GetStatus(ctx context.Context, hash string) (DownloadTask, error)
	Delete(ctx context.Context, hash string, deleteFiles bool) error
	IsAvailable(ctx context.Context) bool
}

type MetadataProvider interface {
	Name() string
	SearchAnime(ctx context.Context, title string) ([]Anime, error)
	GetAnime(ctx context.Context, id string) (Anime, error)
	GetEpisodes(ctx context.Context, animeID string, season int) ([]Episode, error)
	GetCollections(ctx context.Context, username string) ([]Anime, error)
}

type Organizer interface {
	Name() string
	Organize(ctx context.Context, filePath string, anime Anime, episode Episode) (newPath string, err error)
}

type Notifier interface {
	Name() string
	Send(ctx context.Context, title, message string) error
}

// SubscriptionID 订阅唯一标识，用于精确取消订阅
type SubscriptionID uint64

type EventHandler func(event Event)

type EventBus interface {
	Publish(event Event)
	Subscribe(eventType string, handler EventHandler) SubscriptionID
	Unsubscribe(eventType string, id SubscriptionID)
}

const (
	EventSubscriptionAdded   = "subscription.added"
	EventSubscriptionRemoved = "subscription.removed"
	EventDownloadStarted     = "download.started"
	EventDownloadProgress    = "download.progress"
	EventDownloadCompleted   = "download.completed"
	EventDownloadFailed      = "download.failed"
	EventFileOrganized       = "file.organized"
	EventEpisodeIdentified   = "episode.identified"
	EventAnimeMatched        = "anime.matched"
	EventSupplementTriggered = "supplement.triggered"
	EventSupplementCompleted = "supplement.completed"
	EventTaskParsed          = "task.parsed"
)

type ParseResult struct {
	Action       string   `json:"action"`       // subscribe / unsubscribe / list / search / unknown
	Title        string   `json:"title"`        // 番剧标题
	Season       int      `json:"season"`       // 季号 (0 表示未指定)
	Resolution   string   `json:"resolution"`   // 1080p / 720p / 4K
	SubgroupPref string   `json:"subgroup_pref"` // 字幕组偏好
	Keywords     []string `json:"keywords"`     // 额外关键词
	Confidence   float64  `json:"confidence"`   // 0-1
	RawInput     string   `json:"raw_input"`    // 原始输入
}

type TaskParser interface {
	Parse(ctx context.Context, input string) (ParseResult, error)
	Name() string
}

const (
	RSSModeClassic  = "classic"
	RSSModePersonal = "personal"
)

// NormalizeTitle 用于增强番剧标题匹配的鲁棒性（去除空格、统一简繁/变体等）
func NormalizeTitle(s string) string {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "　", "")
	s = strings.ReplaceAll(s, "：", ":")
	s = strings.ReplaceAll(s, "坊", "房") // 统一常见歧义字
	s = strings.ReplaceAll(s, "・", "")
	return strings.ToLower(s)
}
