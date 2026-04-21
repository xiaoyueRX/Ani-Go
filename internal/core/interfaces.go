package core

import (
	"context"
	"time"
)

type TorrentItem struct {
	Title       string
	URL         string
	MagnetURL   string
	InfoHash    string
	Size        int64
	PublishedAt time.Time
	SourceName  string
}

type Anime struct {
	ID          string
	Provider    string
	TitleCN     string
	TitleEN     string
	TitleJP     string
	Year        int
	Season      int
	TotalEps    int
	Type        string
	Description string
	CoverURL    string
	SeriesID    string
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
	Hash      string
	Name      string
	SavePath  string
	Status    string
	Progress  float32
	SpeedDown int64
	Size      int64
	Done      int64
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

type EventBus interface {
	Publish(event Event)
	Subscribe(eventType string, handler EventHandler)
	Unsubscribe(eventType string, handler EventHandler)
}

type EventHandler func(event Event)

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
)
