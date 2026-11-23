package database

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/magnet"
	"gorm.io/gorm"
)

// 旧下载器没有持久化「只包含这一个视频」。仅识别顶层文件名明确等于订阅片名的绑定记录。
var movieReleaseTags = regexp.MustCompile(`(?i)(?:\s*\[(?:movie|剧场版|劇場版|[0-9]{3,4}[pi]|4k|8k)\])+$`)

func movieType(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "movie", "film", "剧场版", "劇場版":
		return true
	}
	return false
}

func legacyMovieRelease(ep Episode, sub Subscription) bool {
	if !ep.HasBoundReviewFile() || strings.ContainsAny(ep.SourceFile, `/\`) || !magnet.IsHexInfoHash(ep.TorrentHash) ||
		ep.ReservationKey == nil || *ep.ReservationKey != EpisodeFileReservationKey(sub.ID, ep.TorrentHash, ep.SourceFile) {
		return false
	}
	ext := filepath.Ext(ep.SourceFile)
	stem := strings.TrimSuffix(ep.SourceFile, ext)
	if !strings.EqualFold(ep.OriginalName, stem) && !strings.EqualFold(ep.OriginalName, ep.SourceFile) {
		return false
	}
	titles := []string{sub.TitleCN, sub.TitleJP, sub.TitleEN}
	if core.MovieExtra(ep.SourceFile, titles...) || core.MovieExtra(ep.OriginalName, titles...) {
		return false
	}
	if ep.GroupName != "" {
		stem = strings.TrimSpace(strings.TrimPrefix(stem, "["+ep.GroupName+"]"))
	}
	stem = strings.TrimSpace(movieReleaseTags.ReplaceAllString(stem, ""))
	for _, name := range titles {
		if stem != "" && strings.EqualFold(stem, strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

// RecoverLegacyMovieFeatures 供旧库迁移和旧备份恢复共用；仅处理调用方提供的一批历史记录。
// 归档不包含源文件修改时间，保守匹配仅使用归档确实保存的证据。
func RecoverLegacyMovieFeatures(tx *gorm.DB, sub Subscription, rows []Episode, fromArchive bool) (bool, error) {
	if !movieType(sub.AnimeType) {
		return false, nil
	}
	changed := false
	for _, ep := range rows {
		if ep.SubscriptionID != sub.ID || ep.Status != "organized" || ep.Number != 0 || ep.OrganizedAt == nil || ep.FileSize <= 0 ||
			!core.IsVideoFile(ep.FinalPath) || !core.IsVideoFile(ep.SourceFile) || (!fromArchive && ep.SourceModTime == 0) {
			continue
		}
		q := tx.Model(&Episode{}).Where("id = ? AND COALESCE(movie_feature, 0) = 0", ep.ID)
		if !ep.HasConfirmedIdentity(ReviewMovieSelection, ep.FileSize, ep.SourceModTime) {
			if ep.ConfirmedIdentity != "" || ep.ReviewKind != "" || ep.ReviewReason != "" || !legacyMovieRelease(ep, sub) {
				continue
			}
			// 已删除的兄弟行同样表明它是多文件任务，不能自动认定正片。
			q = q.Where(`NOT EXISTS (SELECT 1 FROM episodes AS sibling WHERE sibling.id <> episodes.id
				AND sibling.subscription_id = episodes.subscription_id AND sibling.torrent_hash = episodes.torrent_hash COLLATE NOCASE)`)
		}
		result := q.UpdateColumn("movie_feature", true)
		if result.Error != nil {
			return false, result.Error
		}
		changed = changed || result.RowsAffected != 0
	}
	return changed, nil
}

func migrateMovieFeatures(tx *gorm.DB) error {
	var subs []Subscription
	if err := tx.Where(movieTypeSQL).Find(&subs).Error; err != nil {
		return err
	}
	for _, sub := range subs {
		var rows []Episode
		changed := false
		err := tx.Where("subscription_id = ? AND status = 'organized' AND number = 0 AND organized_at IS NOT NULL", sub.ID).
			FindInBatches(&rows, 200, func(_ *gorm.DB, _ int) error {
				found, err := RecoverLegacyMovieFeatures(tx, sub, rows, false)
				changed = changed || found
				return err
			}).Error
		if err != nil {
			return err
		}
		if changed && sub.TotalEpisodes == 1 {
			if _, err := SyncEpisodeProgress(tx, sub.ID, sub.Enabled == nil || *sub.Enabled); err != nil {
				return err
			}
		}
	}
	return nil
}
