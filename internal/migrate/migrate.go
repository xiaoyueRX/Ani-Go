package migrate

import (
	"fmt"
	"log"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

// SourceType 源数据类型
type SourceType string

const (
	SourceAutoBangumi SourceType = "autobangumi"
	SourceAniRSS      SourceType = "anirss"
)

// Stats 迁移统计
type Stats struct {
	Subscriptions int
	Episodes      int
	Downloads     int
	Errors        []string
}

// abAnime AutoBangumi 的 anime 表结构
type abAnime struct {
	ID            uint   `gorm:"primaryKey"`
	Title         string `gorm:"column:title"`
	TmdbID        string `gorm:"column:tmdb_id"`
	BgmID         string `gorm:"column:bgmid"`
	Season        int    `gorm:"column:season"`
	Type          string `gorm:"column:type"`
	Offset        int    `gorm:"column:offset"`
	Filter        string `gorm:"column:filter"`
	Enable        int    `gorm:"column:enable"`
	TotalEpisodes int    `gorm:"column:total_episodes"`
	Directory     string `gorm:"column:directory"`
	CreatedAt     string `gorm:"column:created_at"`
	UpdatedAt     string `gorm:"column:updated_at"`
}

func (abAnime) TableName() string { return "anime" }

// abEpisode AutoBangumi 的 episode 表结构
type abEpisode struct {
	ID            uint    `gorm:"primaryKey"`
	AnimeID       uint    `gorm:"column:anime_id"`
	Title         string  `gorm:"column:title"`
	Season        int     `gorm:"column:season"`
	EpisodeNumber float32 `gorm:"column:episode_number"`
	TorrentHash   string  `gorm:"column:torrent_hash"`
	TorrentName   string  `gorm:"column:torrent_name"`
	TorrentURL    string  `gorm:"column:torrent_url"`
	Status        string  `gorm:"column:status"`
	FileSize      int64   `gorm:"column:file_size"`
	CreatedAt     string  `gorm:"column:created_at"`
	UpdatedAt     string  `gorm:"column:updated_at"`
}

func (abEpisode) TableName() string { return "episode" }

// MigrateFrom 从源数据库迁移数据，自动检测格式
func MigrateFrom(sourceDB *gorm.DB) (*Stats, error) {
	st := &Stats{}

	// 检测源数据类型
	sourceType := detectSource(sourceDB)
	log.Printf("📦 检测到源数据格式: %s", sourceType)

	switch sourceType {
	case SourceAutoBangumi:
		migrateAutoBangumi(sourceDB, st)
	default:
		return st, fmt.Errorf("不支持的源数据格式: %s", sourceType)
	}

	log.Printf("📦 迁移完成: %d 订阅, %d 剧集, %d 下载记录", st.Subscriptions, st.Episodes, st.Downloads)
	if len(st.Errors) > 0 {
		for _, e := range st.Errors {
			log.Printf("⚠️  迁移警告: %s", e)
		}
	}

	return st, nil
}

func detectSource(db *gorm.DB) SourceType {
	if db.Migrator().HasTable("anime") && db.Migrator().HasTable("episode") {
		return SourceAutoBangumi
	}
	return SourceAniRSS
}

func migrateAutoBangumi(source *gorm.DB, st *Stats) {
	var abAnimes []abAnime
	if err := source.Find(&abAnimes).Error; err != nil {
		st.Errors = append(st.Errors, fmt.Sprintf("读取 anime 表失败: %v", err))
		return
	}

	idMap := make(map[uint]uint) // old ID → new ID

	for _, a := range abAnimes {
		enabled := a.Enable != 0
		sub := database.Subscription{
			TitleCN:   a.Title,
			Season:    a.Season,
			BangumiID: a.BgmID,
			MetadataID: a.TmdbID,
			AnimeType: mapABType(a.Type),
			TotalEpisodes: a.TotalEpisodes,
			Enabled:    enabled,
			FilterJSON: a.Filter,
			CustomPath: a.Directory,
			SourceName: "AutoBangumi-Migrated",
		}
		if sub.Season == 0 {
			sub.Season = 1
		}

		if err := database.DB.Create(&sub).Error; err != nil {
			st.Errors = append(st.Errors, fmt.Sprintf("创建订阅 [%s] 失败: %v", a.Title, err))
			continue
		}
		idMap[a.ID] = sub.ID
		st.Subscriptions++
	}

	// 迁移剧集
	var abEpisodes []abEpisode
	if err := source.Find(&abEpisodes).Error; err != nil {
		st.Errors = append(st.Errors, fmt.Sprintf("读取 episode 表失败: %v", err))
		return
	}

	for _, e := range abEpisodes {
		subID, ok := idMap[e.AnimeID]
		if !ok {
			continue
		}

		ep := database.Episode{
			SubscriptionID: subID,
			Season:        e.Season,
			Number:        e.EpisodeNumber,
			Title:         e.Title,
			Status:        mapABEpisodeStatus(e.Status),
			TorrentHash:   e.TorrentHash,
			TorrentURL:    e.TorrentURL,
			OriginalName:  e.TorrentName,
			FileSize:      e.FileSize,
		}
		if ep.Season == 0 {
			ep.Season = 1
		}

		if err := database.DB.Create(&ep).Error; err != nil {
			st.Errors = append(st.Errors, fmt.Sprintf("创建剧集 [%s] 失败: %v", e.Title, err))
			continue
		}
		st.Episodes++

		// 记录下载
		if e.TorrentURL != "" {
			rec := database.DownloadRecord{
				TorrentHash: e.TorrentHash,
				TorrentURL:  e.TorrentURL,
				SourceName:  "AutoBangumi-Migrated",
				AddedAt:     time.Now(),
			}
			database.DB.Where("torrent_hash = ?", e.TorrentHash).FirstOrCreate(&rec)
			st.Downloads++
		}
	}
}

func mapABType(t string) string {
	switch t {
	case "tv", "TV":
		return "TV"
	case "movie", "Movie", "MOVIE":
		return "Movie"
	case "ova", "OVA":
		return "OVA"
	default:
		return "TV"
	}
}

func mapABEpisodeStatus(s string) string {
	switch s {
	case "downloaded", "organized":
		return "downloaded"
	case "downloading":
		return "downloading"
	default:
		return "pending"
	}
}

// MigrateFromPath 从文件路径打开源数据库并迁移
func MigrateFromPath(sourcePath string) (*Stats, error) {
	sourceDB, err := gorm.Open(sqlite.Open(sourcePath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("无法打开源数据库: %w", err)
	}
	return MigrateFrom(sourceDB)
}
