package scheduler

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"gorm.io/gorm"
)

var errSubscriptionRemoved = errors.New("订阅或剧集已删除")

// 同一数据库中的所有入口先取得剧集投递权，再调用外部下载器。
func (s *Scheduler) enqueue(ctx context.Context, sub database.Subscription, item core.TorrentItem, season int, number float32) (bool, error) {
	if s.downloader == nil {
		return false, errors.New("下载器未配置")
	}
	origin := strings.TrimSpace(item.InfoHash)
	if origin != "" {
		origin = strings.ToLower(origin)
	} else {
		uri := item.URL
		if uri == "" {
			uri = item.MagnetURL
		}
		if uri == "" {
			return false, errors.New("缺少下载地址")
		}
		origin = fmt.Sprintf("url:%x", md5.Sum([]byte(uri)))
	}
	key := fmt.Sprintf("%d/%s", sub.ID, origin)
	if number > 0 {
		key = fmt.Sprintf("%d/%d/%g", sub.ID, season, number)
	}
	now := time.Now()
	ep := database.Episode{SubscriptionID: sub.ID, Season: season, Number: number, Title: item.Title, OriginalName: item.Title, Status: "submitting", TorrentHash: item.InfoHash, TorrentURL: item.URL, MagnetURL: item.MagnetURL, OriginKey: origin, ReservationKey: &key, GroupName: item.GroupName, Resolution: item.Resolution, FileSize: item.Size, DownloadStartedAt: &now}
	var reserved bool
	err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current database.Subscription
		r := tx.Where("id = ? AND enabled = ?", sub.ID, true).Limit(1).Find(&current)
		if r.Error != nil || r.RowsAffected == 0 {
			return r.Error
		}
		sub = current
		query := tx.Model(&database.Episode{}).Where("origin_key = ? OR torrent_hash = ?", origin, origin)
		if item.URL != "" {
			query = query.Or("torrent_url = ?", item.URL)
		}
		if number > 0 {
			query = query.Or("(subscription_id = ? AND season = ? AND number = ?)", sub.ID, season, number)
		}
		var count int64
		if err := query.Count(&count).Error; err != nil || count > 0 {
			return err
		}
		if err := tx.Create(&ep).Error; err != nil {
			return err
		}
		reserved = true
		return nil
	})
	if err != nil || !reserved {
		return false, err
	}
	err = s.submitEpisode(ctx, sub, ep, item)
	if errors.Is(err, errSubscriptionRemoved) {
		return false, nil
	}
	return err == nil, err
}

func (s *Scheduler) submitEpisode(ctx context.Context, sub database.Subscription, ep database.Episode, item core.TorrentItem) error {
	item.TrackingTag = trackingTag(ep.OriginKey)
	var receipt string
	var err error
	if sub.CustomPath == "" {
		// 保留媒体库卷的空间检查，下载器可以在另一台机器上使用自己的目录。
		err = downloader.CheckDiskSpaceGuard(s.getTVBasePath())
	}
	if err == nil {
		if tracked, ok := s.downloader.(core.TrackedDownloader); ok {
			receipt, err = tracked.AddWithID(ctx, item, sub.CustomPath)
		} else {
			err = s.downloader.Add(ctx, item, sub.CustomPath)
		}
	}
	if err != nil {
		// 保留重试所需的地址，进程重启也能继续处理。
		dbErr := database.DB.Model(&database.Episode{}).Where("id = ? AND status = ?", ep.ID, "submitting").Update("status", "retrying").Error
		s.publishDownloadFailure(item, sub.ID, err)
		return errors.Join(err, dbErr)
	}
	removed := false
	dbErr := database.DB.Transaction(func(tx *gorm.DB) error {
		var tracked int64
		if err := tx.Model(&database.Subscription{}).Where("id = ?", sub.ID).Count(&tracked).Error; err != nil {
			return err
		}
		if tracked == 0 {
			removed = true
			return tx.Delete(&database.Episode{}, ep.ID).Error
		}
		if err := tx.Model(&database.Episode{}).Where("id = ?", ep.ID).Count(&tracked).Error; err != nil {
			return err
		}
		if tracked == 0 {
			removed = true
			return nil
		}
		updates := map[string]any{"status": "downloading"}
		if unboundHash(ep.TorrentHash) && receipt != "" {
			updates["torrent_hash"] = receipt
		}
		if err := tx.Model(&database.Episode{}).Where("id = ? AND status = ?", ep.ID, "submitting").Updates(updates).Error; err != nil {
			return err
		}
		rec := database.DownloadRecord{TorrentHash: ep.OriginKey, TorrentURL: item.URL, SourceName: item.SourceName, AddedAt: time.Now()}
		return tx.Where("torrent_hash = ?", ep.OriginKey).FirstOrCreate(&rec).Error
	})
	if dbErr == nil && removed {
		return errSubscriptionRemoved
	}
	return dbErr
}

// 已提交但尚未落库的任务优先与下载器对账；只有列表中没有相符任务才重试。
func (s *Scheduler) retryEpisode(ctx context.Context, ep database.Episode) error {
	var sub database.Subscription
	claimed := false
	err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := tx.Where("enabled = ?", true).Limit(1).Find(&sub, ep.SubscriptionID)
		if r.Error != nil || r.RowsAffected == 0 {
			return r.Error
		}
		r = tx.Model(&database.Episode{}).Where("id = ? AND status = ? AND updated_at = ?", ep.ID, ep.Status, ep.UpdatedAt).Updates(map[string]any{"status": "submitting", "updated_at": time.Now()})
		claimed = r.RowsAffected == 1
		return r.Error
	})
	if err != nil || !claimed {
		return err
	}
	return s.submitEpisode(ctx, sub, ep, core.TorrentItem{Title: ep.Title, URL: ep.TorrentURL, MagnetURL: ep.MagnetURL, InfoHash: ep.TorrentHash, GroupName: ep.GroupName, Resolution: ep.Resolution, Size: ep.FileSize, SourceName: sub.SourceName})
}

func mediaName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if mediaFile(name) {
		return strings.TrimSuffix(name, filepath.Ext(name))
	}
	return name
}

func unboundHash(hash string) bool { return hash == "" || strings.HasPrefix(hash, "url:") }

func trackingTag(origin string) string {
	if origin == "" {
		return ""
	}
	return fmt.Sprintf("anigo-id-%x", md5.Sum([]byte(origin)))
}

type downloadIndex struct {
	byTag          map[string]core.DownloadTask
	byHash         map[string]core.DownloadTask
	byName         map[string][]core.DownloadTask
	episodesByName map[string]int
}

func newDownloadIndex(tasks []core.DownloadTask, episodes []database.Episode) downloadIndex {
	idx := downloadIndex{byTag: make(map[string]core.DownloadTask), byHash: make(map[string]core.DownloadTask), byName: make(map[string][]core.DownloadTask), episodesByName: make(map[string]int)}
	for _, t := range tasks {
		if unboundHash(t.Hash) {
			continue
		}
		idx.byHash[strings.ToLower(t.Hash)] = t
		if t.ClientID != "" {
			idx.byHash[strings.ToLower(t.ClientID)] = t
		}
		if t.TrackingTag != "" {
			idx.byTag[t.TrackingTag] = t
		}
		n := mediaName(t.Name)
		if n != "" {
			idx.byName[n] = append(idx.byName[n], t)
		}
		f := mediaName(filepath.Base(t.ContentPath))
		if t.ContentPath != "" && f != n {
			idx.byName[f] = append(idx.byName[f], t)
		}
	}
	for _, ep := range episodes {
		if unboundHash(ep.TorrentHash) {
			idx.episodesByName[mediaName(ep.OriginalName)]++
		}
	}
	return idx
}

func (idx downloadIndex) match(ep database.Episode) (core.DownloadTask, bool) {
	if !unboundHash(ep.TorrentHash) {
		t, ok := idx.byHash[strings.ToLower(ep.TorrentHash)]
		if ok && len(t.FollowedBy) > 0 {
			// HTTP 种子/磁力元数据任务完成后，Aria2 会另建实际下载任务。
			if len(t.FollowedBy) != 1 {
				return core.DownloadTask{}, false
			}
			t, ok = idx.byHash[strings.ToLower(t.FollowedBy[0])]
		}
		return t, ok
	}
	if t, ok := idx.byTag[trackingTag(ep.OriginKey)]; ok {
		return t, true
	}
	n := mediaName(ep.OriginalName)
	candidates := idx.byName[n]
	if n != "" && len(candidates) == 1 && idx.episodesByName[n] == 1 {
		return candidates[0], true
	}
	return core.DownloadTask{}, false
}
