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
	SubgroupID       string
	MetadataID       string
	MetadataProvider string
	CoverURL         string
	Description      string `gorm:"type:text"`
	AnimeType        string
	TotalEpisodes    int
	CurrentEpisodes  int
	Enabled          bool   `gorm:"default:true"`
	Completed        bool   `gorm:"default:false"`
	FilterJSON       string `gorm:"type:text"`
	CustomPath       string
	SeriesID         string
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
