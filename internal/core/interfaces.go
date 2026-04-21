// Package core 定义了 AutoAni 的全部核心接口。
//
// 设计原则：
//   - 功能实现不写在这里，只写"协议"（接口）
//   - 任何人实现这些接口，都能无缝扩展系统
//   - 主程序只依赖接口，不依赖具体实现
//
// 类比：这里是 USB 接口标准，具体的鼠标键盘是另外的包
package core

import (
	"context"
	"time"
)

// ============================================================
// 数据结构（全系统共享的数据类型）
// ============================================================

// TorrentItem 代表一个种子资源条目
type TorrentItem struct {
	Title       string    // 种子名称，如 "[LoliHouse] 末日后酒店 - 01 [WebRip]"
	URL         string    // .torrent 文件下载地址
	MagnetURL   string    // 磁力链接（备用）
	InfoHash    string    // 种子哈希值（唯一ID）
	Size        int64     // 文件大小（字节）
	PublishedAt time.Time // 发布时间
	SourceName  string    // 来自哪个资源站，如 "mikan"
}

// Anime 代表一部番剧的完整信息
type Anime struct {
	ID          string // 在元数据库中的 ID（如 TMDB: "12345"）
	Provider    string // 元数据来源，如 "tmdb" 或 "bgmtv"
	TitleCN     string // 中文名，如 "迷宫饭"
	TitleEN     string // 英文名，如 "Dungeon Meshi"
	TitleJP     string // 日文名，如 "ダンジョン飯"
	Year        int    // 首播年份
	Season      int    // 第几季
	TotalEps    int    // 总集数
	Type        string // 类型："tv" / "movie" / "ova" / "special"
	Description string // 简介
	CoverURL    string // 封面图片 URL
	SeriesID    string // 所属系列 ID（用于同系列归目录）
}

// Episode 代表一集的信息
type Episode struct {
	AnimeID    string  // 属于哪部番剧
	Season     int     // 第几季
	Number     float32 // 第几集（float 支持 12.5 这种特别篇）
	Title      string  // 集标题
	AiredAt    time.Time
}

// Subscription 代表用户的一个追番订阅
type Subscription struct {
	ID            uint
	Title         string   // 番剧名
	BangumiID     string   // Mikan 番剧 ID
	AnimeID       string   // 元数据库 ID
	SourceName    string   // 首选资源站
	SubgroupPref  []string // 字幕组优先级列表
	DownloadPath  string   // 自定义下载路径（空=用全局模板）
	Filter        Filter   // 过滤规则
	Enabled       bool
	Completed     bool
}

// Filter 过滤规则（决定哪些资源要下载）
type Filter struct {
	IncludeKeywords []string // 必须包含的关键词（如 "1080p"）
	ExcludeKeywords []string // 必须排除的关键词（如 "720p"）
	PreferSubgroup  string   // 优先字幕组名
	Resolution      string   // 分辨率要求，如 "1080p"
}

// DownloadTask 代表下载器中一个任务的状态
type DownloadTask struct {
	Hash        string
	Name        string
	SavePath    string
	Status      string  // "downloading" / "completed" / "paused" / "error"
	Progress    float32 // 0.0 - 1.0
	SpeedDown   int64   // 下载速度（字节/秒）
	Size        int64   // 总大小
	Done        int64   // 已下载
}

// Event 是事件总线上流转的事件
type Event struct {
	Type    string                 // 事件类型，如 "download.complete"
	Payload map[string]interface{} // 事件数据
	Time    time.Time
}

// ============================================================
// 核心接口（Source / Downloader / Metadata / Organizer / Notifier）
// ============================================================

// Source 资源站接口
// 实现这个接口 = 支持一个新的种子来源
//
// 示例：
//   - MikanSource：从 Mikan Project 获取资源
//   - NyaaSource：从 Nyaa.si 获取资源
//   - 你自定义的 XXXSource：任意资源站
type Source interface {
	// Name 返回资源站唯一标识，如 "mikan", "nyaa"
	Name() string

	// FetchRSS 拉取 RSS 链接中的最新种子
	FetchRSS(ctx context.Context, url string) ([]TorrentItem, error)

	// SearchAnime 在资源站搜索番剧，返回相关种子
	SearchAnime(ctx context.Context, title string) ([]TorrentItem, error)

	// FetchHistory 获取某个番剧的全部历史种子（补全老番用）
	// bangumiID 是资源站的番剧标识
	FetchHistory(ctx context.Context, bangumiID string, filter Filter) ([]TorrentItem, error)

	// IsAvailable 检查该资源站当前是否可达
	IsAvailable(ctx context.Context) bool
}

// Downloader 下载器接口
// 实现这个接口 = 支持一个新的下载工具
//
// 示例：
//   - QBittorrentDownloader
//   - TransmissionDownloader
//   - Aria2Downloader
type Downloader interface {
	// Name 返回下载器唯一标识，如 "qbittorrent"
	Name() string

	// Add 添加一个种子到下载队列
	// savePath 是希望保存到的目录
	Add(ctx context.Context, item TorrentItem, savePath string) error

	// List 列出所有下载任务
	List(ctx context.Context) ([]DownloadTask, error)

	// GetStatus 查询某个种子的状态
	GetStatus(ctx context.Context, hash string) (DownloadTask, error)

	// Delete 删除任务（deleteFiles=true 同时删文件）
	Delete(ctx context.Context, hash string, deleteFiles bool) error

	// IsAvailable 检查下载器是否在线
	IsAvailable(ctx context.Context) bool
}

// MetadataProvider 元数据接口
// 实现这个接口 = 支持一个新的番剧数据库
//
// 示例：
//   - TMDBProvider
//   - BGMTVProvider（番组计划）
//   - AniListProvider
type MetadataProvider interface {
	// Name 返回元数据源唯一标识，如 "tmdb", "bgmtv"
	Name() string

	// SearchAnime 根据标题搜索番剧
	SearchAnime(ctx context.Context, title string) ([]Anime, error)

	// GetAnime 获取番剧详细信息
	GetAnime(ctx context.Context, id string) (Anime, error)

	// GetEpisodes 获取某季的所有集数信息
	GetEpisodes(ctx context.Context, animeID string, season int) ([]Episode, error)
}

// Organizer 文件整理接口
// 实现这个接口 = 完全自定义文件整理逻辑
type Organizer interface {
	// Name 返回整理器唯一标识
	Name() string

	// Organize 对一个下载完成的文件进行整理（移动+重命名）
	// filePath 是当前文件路径
	// anime/episode 是匹配到的元数据
	Organize(ctx context.Context, filePath string, anime Anime, episode Episode) (newPath string, err error)
}

// Notifier 通知接口
// 实现这个接口 = 支持一个新的通知渠道
//
// 示例：
//   - TelegramNotifier
//   - WebhookNotifier
type Notifier interface {
	// Name 返回通知渠道唯一标识，如 "telegram"
	Name() string

	// Send 发送通知
	Send(ctx context.Context, title, message string) error
}

// ============================================================
// 扩展点：事件总线
// ============================================================

// EventHandler 是事件处理函数类型
type EventHandler func(event Event)

// EventBus 事件总线接口
// 插件通过它监听系统内发生的一切
type EventBus interface {
	// Publish 发布一个事件（异步，不阻塞）
	Publish(event Event)

	// Subscribe 订阅某类事件（eventType 支持通配符，如 "download.*"）
	Subscribe(eventType string, handler EventHandler)

	// Unsubscribe 取消订阅
	Unsubscribe(eventType string, handler EventHandler)
}

// ============================================================
// 事件类型常量（系统内所有事件的名称）
// 插件可以监听任意这些事件
// ============================================================

const (
	EventSubscriptionAdded    = "subscription.added"
	EventSubscriptionRemoved  = "subscription.removed"
	EventDownloadStarted      = "download.started"
	EventDownloadProgress     = "download.progress"
	EventDownloadCompleted    = "download.completed"
	EventDownloadFailed       = "download.failed"
	EventFileOrganized        = "file.organized"
	EventEpisodeIdentified    = "episode.identified"
	EventAnimeMatched         = "anime.matched"
	EventSupplementTriggered  = "supplement.triggered"
	EventSupplementCompleted  = "supplement.completed"
)
