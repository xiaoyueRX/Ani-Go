package database

import (
	"time"

	"gorm.io/gorm"
)

// ============================================================
// Subscription - 追番订阅表
// ============================================================

// Subscription 存储用户订阅的每部番剧
// 标签 `gorm:"..."` 告诉 GORM 怎么建数据库列
type Subscription struct {
	gorm.Model // 内嵌：自动添加 ID, CreatedAt, UpdatedAt, DeletedAt 字段

	// 番剧基本信息
	TitleCN   string `gorm:"not null"` // 中文名
	TitleEN   string                   // 英文名
	TitleJP   string                   // 日文名
	Year      int                      // 首播年份
	Season    int    `gorm:"default:1"` // 第几季

	// 来源信息
	SourceName string // 资源站，如 "mikan"
	BangumiID  string // 资源站的番剧 ID（Mikan bangumiId）
	RSSURL     string // 订阅的 RSS URL

	// 字幕组信息
	SubgroupName string // 选择的字幕组名，如 "LoliHouse"
	SubgroupID   string // 字幕组 ID（Mikan subgroupid）

	// 元数据
	MetadataID       string // TMDB/BGM.tv 的番剧 ID
	MetadataProvider string // 使用哪个元数据源
	CoverURL         string // 封面图片 URL
	Description      string `gorm:"type:text"` // 番剧简介
	AnimeType        string // 类型："tv" / "movie" / "ova" / "special"
	TotalEpisodes    int    // 总集数（从元数据获取）

	// 下载状态
	CurrentEpisodes int  // 已下载集数
	Enabled         bool `gorm:"default:true"` // 是否启用追番
	Completed       bool `gorm:"default:false"` // 是否已完结

	// 过滤规则（JSON 存储，方便扩展）
	FilterJSON string `gorm:"type:text"` // 序列化后的过滤规则

	// 下载路径（留空=使用全局模板）
	CustomPath string

	// 所属系列（用于同系列归目录）
	SeriesID string // TMDB Collection ID 或 BGM.tv 系列 ID
}

// ============================================================
// Episode - 剧集追踪表
// ============================================================

// Episode 记录每一集的下载状态
type Episode struct {
	gorm.Model

	SubscriptionID uint   `gorm:"not null;index"` // 属于哪个订阅（外键）
	Season         int    `gorm:"default:1"`
	Number         float32 `gorm:"not null"` // 集数（float 支持 12.5 特别篇）
	Title          string // 集标题

	// 下载状态
	Status      string `gorm:"default:'pending'"` // pending/downloading/completed/failed
	TorrentHash string `gorm:"uniqueIndex"`        // 种子哈希（用于防重复）
	TorrentURL  string // 种子下载地址

	// 文件信息（整理完成后填入）
	OriginalName string // 下载时的原始文件名
	FinalPath    string // 整理后的最终路径
	FileSize     int64  // 文件大小（字节）

	// 时间
	DownloadStartedAt  *time.Time
	DownloadFinishedAt *time.Time
	OrganizedAt        *time.Time
}

// ============================================================
// DownloadRecord - 下载历史（防止重复下载）
// ============================================================

// DownloadRecord 记录所有曾经发起过的下载
// 核心作用：防止同一个种子被重复添加到下载器
type DownloadRecord struct {
	gorm.Model

	TorrentHash string `gorm:"uniqueIndex;not null"` // 种子唯一标识
	TorrentURL  string
	SourceName  string    // 来自哪个资源站
	AddedAt     time.Time // 添加时间
}

// ============================================================
// Setting - 用户设置表
// ============================================================

// Setting 是键值对存储，用于 Web UI 可以修改的所有设置
// 好处：不需要重启程序就能改配置
type Setting struct {
	Key   string `gorm:"primaryKey"` // 设置项的键
	Value string `gorm:"type:text"`  // 设置项的值（JSON 字符串）
}
