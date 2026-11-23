package migrate

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	Skipped       int
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
	if detectSource(sourceDB) != SourceAutoBangumi {
		return nil, fmt.Errorf("不支持此 SQLite 结构，需要 AutoBangumi 的 anime 和 episode 表")
	}
	var shows []abAnime
	var episodes []abEpisode
	if err := sourceDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Find(&shows).Error; err != nil {
			return err
		}
		return tx.Find(&episodes).Error
	}); err != nil {
		return nil, err
	}
	return writeMigration(func(tx *gorm.DB, st *Stats) error {
		return migrateAutoBangumi(tx, st, shows, episodes)
	})
}

func detectSource(db *gorm.DB) SourceType {
	m := db.Migrator()
	if m.HasTable("anime") && m.HasTable("episode") && m.HasColumn(&abAnime{}, "title") && m.HasColumn(&abEpisode{}, "anime_id") && m.HasColumn(&abEpisode{}, "episode_number") {
		return SourceAutoBangumi
	}
	return ""
}

func writeMigration(write func(*gorm.DB, *Stats) error) (*Stats, error) {
	st := &Stats{}
	if err := database.DB.Transaction(func(tx *gorm.DB) error { return write(tx, st) }); err != nil {
		return nil, fmt.Errorf("导入已回滚: %w", err)
	}
	return st, nil
}

// importSubscription 返回匹配的目标订阅，不覆盖用户对现有记录的修改。
func importSubscription(tx *gorm.DB, st *Stats, sub database.Subscription) (uint, error) {
	if strings.TrimSpace(sub.TitleCN) == "" {
		return 0, fmt.Errorf("订阅缺少标题")
	}
	if sub.Season <= 0 {
		sub.Season = 1
	}
	q := tx.Where("season = ?", sub.Season)
	switch {
	case sub.BGMSubjectID() != "":
		q = q.Where("((LOWER(metadata_provider) = ? AND metadata_id = ?) OR (source_name = ? AND bangumi_id = ? AND title_cn = ?))", "bgm.tv", sub.MetadataID, "AutoBangumi-Migrated", sub.MetadataID, sub.TitleCN)
	case sub.RSSURL != "":
		q = q.Where("rss_url = ? AND title_cn = ?", sub.RSSURL, sub.TitleCN)
	default:
		q = q.Where("title_cn = ? AND subgroup_name = ?", sub.TitleCN, sub.SubgroupName)
	}
	var matches []database.Subscription
	if err := q.Limit(2).Find(&matches).Error; err != nil {
		return 0, err
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("订阅匹配不唯一: %s", sub.TitleCN)
	}
	if len(matches) == 1 {
		st.Skipped++
		return matches[0].ID, nil
	}
	if err := tx.Create(&sub).Error; err != nil {
		return 0, err
	}
	st.Subscriptions++
	return sub.ID, nil
}

func migrateAutoBangumi(tx *gorm.DB, st *Stats, shows []abAnime, episodes []abEpisode) error {
	ids := make(map[uint]uint, len(shows))
	for _, a := range shows {
		enabled := a.Enable != 0
		sub := database.Subscription{
			TitleCN: a.Title, Season: a.Season, TMDBID: a.TmdbID, AnimeType: mapABType(a.Type), TotalEpisodes: a.TotalEpisodes,
			Enabled: &enabled, FilterJSON: a.Filter, CustomPath: a.Directory, SourceName: "AutoBangumi-Migrated",
		}
		if a.BgmID != "" {
			sub.MetadataProvider = "BGM.tv"
			sub.MetadataID = a.BgmID
		} else if a.TmdbID != "" {
			sub.MetadataProvider = "TMDB"
			sub.MetadataID = a.TmdbID
		}
		id, err := importSubscription(tx, st, sub)
		if err != nil {
			return err
		}
		ids[a.ID] = id
	}
	for _, e := range episodes {
		id, ok := ids[e.AnimeID]
		if !ok {
			return fmt.Errorf("剧集 %s 找不到源订阅 %d", e.Title, e.AnimeID)
		}
		ep := database.Episode{SubscriptionID: id, Season: e.Season, Number: e.EpisodeNumber, Title: e.Title, Status: mapABEpisodeStatus(e.Status), TorrentHash: e.TorrentHash, TorrentURL: e.TorrentURL, OriginalName: e.TorrentName, FileSize: e.FileSize}
		if ep.Season == 0 {
			ep.Season = 1
		}
		q := tx.Where("subscription_id = ? AND season = ? AND number = ? AND torrent_hash = ?", id, ep.Season, ep.Number, ep.TorrentHash)
		if ep.TorrentHash == "" {
			q = q.Where("torrent_url = ? AND original_name = ?", ep.TorrentURL, ep.OriginalName)
		}
		var existing database.Episode
		res := q.Limit(1).Find(&existing)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			if err := tx.Create(&ep).Error; err != nil {
				return err
			}
			st.Episodes++
		} else {
			st.Skipped++
		}
		if e.TorrentURL != "" && e.TorrentHash != "" {
			rec := database.DownloadRecord{TorrentHash: e.TorrentHash, TorrentURL: e.TorrentURL, SourceName: "AutoBangumi-Migrated", AddedAt: time.Now()}
			result := tx.Where("torrent_hash = ?", e.TorrentHash).FirstOrCreate(&rec)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				st.Downloads++
			} else {
				st.Skipped++
			}
		}
	}
	return nil
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
	f, err := os.Open(sourcePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	magic, _ := r.Peek(16)
	if bytes.HasPrefix(magic, []byte("SQLite format 3\x00")) {
		abs, err := filepath.Abs(sourcePath)
		if err != nil {
			return nil, err
		}
		path := filepath.ToSlash(abs)
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
		sourceDB, err := gorm.Open(sqlite.Open(u.String()), &gorm.Config{})
		if err != nil {
			return nil, err
		}
		sqlDB, err := sourceDB.DB()
		if err != nil {
			return nil, err
		}
		defer sqlDB.Close()
		return MigrateFrom(sourceDB)
	}
	if bytes.HasPrefix(magic, []byte("PK\x03\x04")) {
		info, err := f.Stat()
		if err != nil {
			return nil, err
		}
		z, err := zip.NewReader(f, info.Size())
		if err != nil {
			return nil, err
		}
		var manifest *zip.File
		for _, file := range z.File {
			if file.Name == "ani.v2.json" {
				if manifest != nil {
					return nil, fmt.Errorf("备份中有重复的 ani.v2.json")
				}
				manifest = file
			}
		}
		if manifest == nil {
			return nil, fmt.Errorf("未找到 ani-rss 订阅清单 ani.v2.json")
		}
		entry, err := manifest.Open()
		if err != nil {
			return nil, err
		}
		defer entry.Close()
		return migrateAniRSS(entry)
	}
	return migrateAniRSS(r)
}

// 官方 ani-rss 使用 ani.v2.json 存储订阅数组。进度字段不包含逐集下载历史。
func migrateAniRSS(r io.Reader) (*Stats, error) {
	const maxSize = 50 << 20
	data, err := io.ReadAll(io.LimitReader(r, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize {
		return nil, fmt.Errorf("订阅清单超过 50 MiB")
	}
	var entries []struct {
		ID                   string
		URL                  string
		Title                string
		JPTitle              string
		Year                 int
		Season               int
		Subgroup             string
		Image                string
		Ova                  bool
		Type                 string
		Enable               *bool
		Completed            bool
		CurrentEpisodeNumber int
		TotalEpisodeNumber   int
		Match                []string
		Exclude              []string
	}
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), &entries); err != nil || entries == nil {
		return nil, fmt.Errorf("需要 AutoBangumi SQLite、ani-rss 的 ani.v2.json 或官方备份 ZIP")
	}
	return writeMigration(func(tx *gorm.DB, st *Stats) error {
		for _, a := range entries {
			if a.ID == "" || a.Title == "" || a.URL == "" {
				return fmt.Errorf("ani-rss 订阅缺少 id、title 或 url")
			}
			kind := mapABType(a.Type)
			if a.Ova {
				kind = "OVA"
			}
			filter, err := json.Marshal(struct {
				Include []string `json:"include_keywords,omitempty"`
				Exclude []string `json:"exclude_keywords,omitempty"`
			}{a.Match, a.Exclude})
			if err != nil {
				return err
			}
			_, err = importSubscription(tx, st, database.Subscription{TitleCN: a.Title, TitleJP: a.JPTitle, Season: a.Season, Year: a.Year, AnimeType: kind, RSSURL: a.URL, Enabled: a.Enable, Completed: a.Completed, CoverURL: a.Image, SubgroupName: a.Subgroup, CurrentEpisodes: a.CurrentEpisodeNumber, TotalEpisodes: a.TotalEpisodeNumber, FilterJSON: string(filter), SourceName: "ani-rss"})
			if err != nil {
				return err
			}
		}
		return nil
	})
}
