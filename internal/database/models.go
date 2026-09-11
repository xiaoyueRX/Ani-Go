package database

import (
	"time"

	"gorm.io/gorm"
)

type Subscription struct {
	gorm.Model
	TitleCN          string `gorm:"not null"`
	TitleEN          string
	TitleJP          string
	Year             int
	Season           int `gorm:"default:1"`
	SourceName       string
	BangumiID        string
	RSSURL           string
	SubgroupName     string
	ExcludedKeywords string `gorm:"type:text;default:''"`
	SkipBulkUpdate   bool   `gorm:"default:false"`
	AllowedSubgroups string `gorm:"type:text;default:'[]'"`
	SubgroupID       string
	MetadataID       string
	MetadataProvider string
	CoverURL         string
	Description      string `gorm:"type:text"`
	AnimeType        string
	TotalEpisodes    int
	CurrentEpisodes  int
	Enabled          *bool  `gorm:"default:true"`
	Completed        bool   `gorm:"default:false"`
	FilterJSON       string `gorm:"type:text"`
	CustomPath       string
	SeriesID         string
	TMDBID            string `gorm:"index"`
	IMDBID            string `gorm:"index"`
	StallTimeoutHours int    `gorm:"default:24"`
}

type Episode struct {
	gorm.Model
	SubscriptionID     uint    `gorm:"not null;index"`
	Season             int     `gorm:"default:1"`
	Number             float32 `gorm:"not null"`
	Title              string
	Status             string `gorm:"default:'pending'"`
	TorrentHash        string `gorm:"uniqueIndex"`
	TorrentURL         string
	OriginalName       string
	FinalPath          string
	FileSize           int64
	GroupName          string
	Resolution         string
	DownloadStartedAt  *time.Time
	DownloadFinishedAt *time.Time
	OrganizedAt        *time.Time
}

type DownloadRecord struct {
	gorm.Model
	TorrentHash string `gorm:"uniqueIndex;not null"`
	TorrentURL  string
	SourceName  string
	AddedAt     time.Time
}

type Setting struct {
	Key   string `gorm:"primaryKey"`
	Value string `gorm:"type:text"`
}

// User 用户模型（鉴权用）
type User struct {
	gorm.Model
	Username     string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"`
	TokenVersion int    `gorm:"default:1"`
	AvatarURL    string `gorm:"default:''"`
}

// NotificationLog 通知投递履历模型
type NotificationLog struct {
	gorm.Model
	EventType  string `gorm:"index;size:64" json:"event_type"`
	Channel    string `gorm:"index;size:64" json:"channel"`
	Title      string `gorm:"size:255" json:"title"`
	Content    string `gorm:"type:text" json:"content"`
	Status     string `gorm:"index;size:32" json:"status"` // success, failed, dlq
	ErrorMsg   string `gorm:"type:text" json:"error_msg"`
	RetryCount int    `gorm:"default:0" json:"retry_count"`
}

// ParserCache 标题解析持久化缓存
type ParserCache struct {
	gorm.Model
	RawTitle   string  `gorm:"uniqueIndex;not null;type:text"`
	CleanTitle string  `gorm:"type:text"`
	Season     int     `gorm:"default:1"`
	Episode    float32 `gorm:"not null"`
	Subgroup   string  `gorm:"size:128"`
	Resolution string  `gorm:"size:32"`
	Confidence float32 `gorm:"default:1.0"`
	ResolvedBy string  `gorm:"size:32"` // regex, airdate, ai
}

