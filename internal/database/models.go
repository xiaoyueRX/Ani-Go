package database

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

type Subscription struct {
	gorm.Model
	TitleCN           string `gorm:"not null"`
	TitleEN           string
	TitleJP           string
	Year              int
	Season            int `gorm:"default:1"`
	SourceName        string
	BangumiID         string
	MikanID           string `gorm:"index"`
	RSSURL            string
	SubgroupName      string
	ExcludedKeywords  string `gorm:"type:text;default:''"`
	SkipBulkUpdate    bool   `gorm:"default:false"`
	AllowedSubgroups  string `gorm:"type:text;default:'[]'"`
	SubgroupID        string
	MetadataID        string
	MetadataProvider  string
	CoverURL          string
	Description       string `gorm:"type:text"`
	AnimeType         string
	TotalEpisodes     int
	CurrentEpisodes   int
	Enabled           *bool  `gorm:"default:true"`
	Completed         bool   `gorm:"default:false"`
	FilterJSON        string `gorm:"type:text"`
	CustomPath        string
	SeriesID          string
	TMDBID            string `gorm:"index"`
	IMDBID            string `gorm:"index"`
	StallTimeoutHours int    `gorm:"default:24"`
}

// BGMSubjectID 只返回已经确认属于 Bangumi.tv 的条目标识。
func (s Subscription) BGMSubjectID() string {
	if strings.EqualFold(s.MetadataProvider, "BGM.tv") {
		return strings.TrimSpace(s.MetadataID)
	}
	return ""
}

type Episode struct {
	gorm.Model
	SubscriptionID     uint `gorm:"not null;index"`
	Season             int
	Number             float32 `gorm:"not null"`
	Title              string
	Status             string  `gorm:"default:'pending'"`
	ManualStatus       string  `json:"ManualStatus,omitempty"`
	ReviewReason       string  `json:"ReviewReason,omitempty"`
	ReviewKind         string  `json:"ReviewKind,omitempty"`
	ConfirmedIdentity  string  `json:"ConfirmedIdentity,omitempty"`
	MovieFeature       *bool   `gorm:"default:false"` // 旧备份缺少正片验证字段时为 nil。
	TorrentHash        string  `gorm:"index:idx_episode_client_hash"`
	OriginKey          string  `gorm:"index"`
	ReservationKey     *string `gorm:"uniqueIndex:idx_episode_claim,where:deleted_at IS NULL"`
	SourceFile         string
	SourceModTime      int64 `json:"-"`
	DownloadRoot       string
	MagnetURL          string
	TorrentURL         string
	OriginalName       string
	MoveSourcePath     string `gorm:"index:idx_episode_move_pending,where:move_source_path <> ''" json:"-"`
	MoveSourceModTime  int64  `json:"-"`
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
