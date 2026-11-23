package scheduler

import (
	"context"
	"os"
	"path/filepath"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
)

func unknownLegacyEpisode(ep database.Episode, sub database.Subscription) bool {
	return !movieSubscription(sub) && !parser.ValidEpisodeIdentity(ep.Season, ep.Number)
}

func legacyBatchCandidate(ep database.Episode) bool {
	return ep.Number == 0 && ep.SourceFile == "" && !unboundHash(ep.TorrentHash) && parser.ExtractEpisodeDetailed(ep.OriginalName).IsBatch
}

func (s *Scheduler) legacyReviewMember(ep database.Episode, sub database.Subscription, reason string) database.Episode {
	ep.Status = database.EpisodeNeedsReview
	ep.ReviewReason = reason
	ep.ReviewKind, ep.ConfirmedIdentity = "", ""
	ep.DownloadRoot, ep.SourceFile = "", ""
	ep.FileSize, ep.SourceModTime = 0, 0
	inspector, ok := s.organizer.(interface {
		LibrarySourceRoot(string, string) (string, bool)
	})
	if ok && mediaFile(ep.FinalPath) {
		if root, safe := inspector.LibrarySourceRoot(sub.AnimeType, ep.FinalPath); safe {
			rel, relErr := filepath.Rel(root, ep.FinalPath)
			info, err := os.Lstat(ep.FinalPath)
			if err == nil && relErr == nil && info.Mode().IsRegular() && info.Size() > 0 && filepath.IsLocal(rel) {
				ep.ReviewKind = database.ReviewEpisodeIdentity
				ep.DownloadRoot, ep.SourceFile = root, rel
				ep.FileSize, ep.SourceModTime = info.Size(), info.ModTime().UnixNano()
				return ep
			}
		}
	}
	ep.ReviewReason += "；请核对历史路径及媒体库挂载后重新验证"
	return ep
}

// 旧库文件保留原路径，只有能核实库根和成员的行才允许用户确认季集。
func (s *Scheduler) holdLegacyEpisode(ctx context.Context, ep database.Episode, sub database.Subscription) error {
	member := s.legacyReviewMember(ep, sub, "旧库文件缺少可验证的季集身份，请核对原文件")
	return ep.OrganizationSnapshot(database.DB.WithContext(ctx)).Updates(map[string]any{
		"status": member.Status, "review_kind": member.ReviewKind, "review_reason": member.ReviewReason, "confirmed_identity": "",
		"source_file": member.SourceFile, "download_root": member.DownloadRoot, "file_size": member.FileSize, "source_mod_time": member.SourceModTime,
	}).Error
}

func (s *Scheduler) recoverLegacyEpisode(ctx context.Context, ep database.Episode, sub database.Subscription) error {
	if legacyBatchCandidate(ep) {
		return ep.OrganizationSnapshot(database.DB.WithContext(ctx)).Update("status", "downloaded").Error
	}
	return s.holdLegacyEpisode(ctx, ep, sub)
}
