package database

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/magnet"
	"gorm.io/gorm"
)

const (
	EpisodeNeedsReview    = "needs_review"
	ReviewEpisodeIdentity = "episode_identity"
	ReviewMovieSelection  = "movie_selection"
)

// EpisodeFileReservationKey 以任务和相对路径区分同一分集的多个发行版本。
func EpisodeFileReservationKey(subscriptionID uint, torrentHash, sourceFile string) string {
	return fileReservationKey(subscriptionID, torrentHash, filepath.ToSlash(sourceFile))
}

// MatchesFileReservation 用源系统原本参与哈希的路径字节校验备份中的成员身份。
func (ep Episode) MatchesFileReservation(sourcePath string) bool {
	return ep.ReservationKey != nil && *ep.ReservationKey == fileReservationKey(ep.SubscriptionID, ep.TorrentHash, sourcePath)
}

func fileReservationKey(subscriptionID uint, torrentHash, sourcePath string) string {
	file := magnet.CanonicalTaskID(torrentHash) + "\x00" + sourcePath
	return fmt.Sprintf("%d/file/%x", subscriptionID, sha256.Sum256([]byte(file)))
}

// HasBoundReviewFile 表示源文件是下载根目录内保存的相对路径。
func (ep Episode) HasBoundReviewFile() bool {
	return ep.DownloadRoot != "" && filepath.IsLocal(ep.SourceFile) && filepath.Clean(ep.SourceFile) != "."
}

// CanRetryOrganization 对身份复核只开放重新核实已绑定源的操作。
func (ep Episode) CanRetryOrganization() bool {
	return ep.Status == EpisodeNeedsReview && (ep.ReviewKind == "" || ((ep.ReviewKind == ReviewMovieSelection || ep.ReviewKind == ReviewEpisodeIdentity) && ep.HasBoundReviewFile()))
}

// ConfirmationSignature 绑定人工确认的类型、订阅、季集与具体源文件的持久证据。
func (ep Episode) ConfirmationSignature(kind string) string {
	if (kind != ReviewEpisodeIdentity && kind != ReviewMovieSelection) || !ep.HasBoundReviewFile() ||
		ep.FileSize <= 0 || ep.SourceModTime == 0 {
		return ""
	}
	identity := fmt.Sprintf("%q/%d/%q/%q/%q/%d/%d/%d/%g", kind, ep.SubscriptionID, magnet.CanonicalTaskID(ep.TorrentHash), ep.DownloadRoot, ep.SourceFile, ep.FileSize, ep.SourceModTime, ep.Season, ep.Number)
	if ep.TorrentHash == "" {
		// 旧版 E00 可没有任务标识。仅允许已保存且精确绑定到原媒体库路径的分集。
		if kind != ReviewEpisodeIdentity || ep.ID == 0 || !filepath.IsAbs(ep.FinalPath) || ep.FinalPath != filepath.Join(ep.DownloadRoot, ep.SourceFile) {
			return ""
		}
		identity += fmt.Sprintf("/legacy/%d/%q", ep.ID, ep.FinalPath)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
}

// HasConfirmedIdentity 在实际整理前再次核对当前文件的大小和修改时间。
func (ep Episode) HasConfirmedIdentity(kind string, size, modTime int64) bool {
	return ep.ConfirmedIdentity != "" && ep.FileSize == size && ep.SourceModTime == modTime && ep.ConfirmedIdentity == ep.ConfirmationSignature(kind)
}

// OrganizationSnapshot 将更新限定在本轮读取的活跃行，避免过期轮询或重复确认改写新状态。
func (ep Episode) OrganizationSnapshot(db *gorm.DB) *gorm.DB {
	q := db.Model(&Episode{}).Where("id = ? AND status = ?", ep.ID, ep.Status)
	if ep.UpdatedAt.IsZero() {
		return q.Where("updated_at IS NULL OR updated_at = ?", ep.UpdatedAt)
	}
	return q.Where("updated_at = ?", ep.UpdatedAt)
}

// MarkEpisodeForReview 仅隔离本次检查的已下载记录。
func MarkEpisodeForReview(ctx context.Context, ep Episode, reason string) (bool, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return false, errors.New("缺少整理复核原因")
	}
	if ep.Status != "downloaded" {
		return false, nil
	}
	q := ep.OrganizationSnapshot(DB.WithContext(ctx))
	changes := map[string]any{"status": EpisodeNeedsReview, "review_reason": reason, "review_kind": ep.ReviewKind}
	if ep.ReviewKind != "" {
		changes["confirmed_identity"] = ""
	}
	updated := q.Updates(changes)
	return updated.RowsAffected == 1, updated.Error
}
