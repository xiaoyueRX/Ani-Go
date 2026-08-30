package core

import (
	"context"
	"time"
)

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
