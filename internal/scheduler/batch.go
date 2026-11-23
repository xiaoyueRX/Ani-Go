package scheduler

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/magnet"

	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
	"gorm.io/gorm"
)

func mediaFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mkv", ".mp4", ".avi", ".rmvb", ".m4v", ".webm", ".ts", ".m2ts", ".mov":
		return true
	}
	return false
}

// 同一任务的每个源文件有独立记录；一项需要复核时，其余成员仍可整理。
func (s *Scheduler) expandBatch(ctx context.Context, parent database.Episode, sub database.Subscription, dir string, selection *verifiedSelection) ([]database.Episode, error) {
	var members []database.Episode
	visit := func(path string, size, modTime int64, issue string) error {
		rel, err := filepath.Rel(dir, path)
		if err != nil || !filepath.IsLocal(rel) {
			return fmt.Errorf("%w: 成员路径越出任务下载目录", errOrganizationReview)
		}
		title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		member := parent
		member.Model = gorm.Model{}
		member.SourceFile = rel
		member.DownloadRoot = dir
		member.Title = title
		member.Status = "downloaded"
		member.OrganizedAt = nil
		member.FinalPath = ""
		member.FileSize = size
		member.SourceModTime = modTime
		key := database.EpisodeFileReservationKey(sub.ID, parent.TorrentHash, rel)
		member.ReservationKey = &key
		member.ReviewKind = ""
		member.ReviewReason = ""
		member.ConfirmedIdentity = ""
		single := selection != nil && len(selection.Files) == 1
		if movieSubscription(sub) {
			if !single || batchEpisodeReviewReason(rel, sub, 0, 0) != "" {
				member.ReviewKind = database.ReviewMovieSelection
				member.Number = 0
				issue = "请确认要整理的电影正片，任务中有多个视频或文件带附加内容标记"
			}
		} else {
			parsed := parser.ExtractEpisodeDetailed(title)
			if parsed.Episode == 0 {
				if n, err := strconv.Atoi(title); err == nil && n > 0 && n <= 2000 {
					parsed.Episode = float32(n)
				}
			}
			member.Season, member.Number = parsed.Season, parsed.Episode
			if !parsed.ExplicitSeason && !parsed.IsSpecial && member.Season == 1 && parent.Season > 1 {
				member.Season = parent.Season
			}
			// 跳过附加内容、明确季号和未知号码，不凭连续数列推断跨季偏移。
			if !parsed.ExplicitSeason && !parsed.IsSpecial && member.Season == sub.Season && member.Season > 1 && sub.TotalEpisodes > 0 &&
				member.Number > float32(sub.TotalEpisodes) && batchEpisodeReviewReason(rel, sub, member.Season, 0) == "" {
				key := fmt.Sprintf("sub/%d/s%d/%s", sub.ID, member.Season, title)
				if cached, ok := cachedSubscriptionEpisode(key, member.Season, sub.TotalEpisodes); ok {
					member.Number = cached.Episode
				}
			}
			if single && member.Number == 0 && parent.Number > 0 && !parsed.IsSpecial {
				member.Season, member.Number = parent.Season, parent.Number
			}
			why := batchEpisodeReviewReason(rel, sub, member.Season, member.Number)
			if why == "" && single && parent.Number > 0 && parsed.Episode > 0 && (member.Season != parent.Season || member.Number != parent.Number) {
				why = "源文件季集与订阅下载时确认的季集不一致"
			}
			if why == "" && !parser.ValidEpisodeIdentity(member.Season, member.Number) {
				why = "文件名不能确定剧集号，请确认季和集"
			}
			if why != "" {
				member.ReviewKind = database.ReviewEpisodeIdentity
				member.Number = 0
				issue = why
			}
		}
		if issue != "" {
			member.Status = database.EpisodeNeedsReview
			member.ReviewReason = issue
		}
		members = append(members, member)
		return nil
	}
	if selection != nil {
		for _, f := range selection.Files {
			if err := visit(f.Path, f.Size, f.ModTime, selection.issues[f.Path]); err != nil {
				return nil, err
			}
		}
	} else {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.Type().IsRegular() || !mediaFile(d.Name()) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			why := ""
			if info.Size() <= 0 {
				why = "源视频文件为空"
			}
			return visit(path, info.Size(), info.ModTime().UnixNano(), why)
		})
		if err != nil {
			return nil, err
		}
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("%w: 合集目录中未找到视频", errOrganizationReview)
	}
	if parent.FinalPath != "" {
		members = s.restoreMovedBatchMember(parent, sub, members)
	}
	return storeBatchMembers(ctx, parent, members)
}

func movieSubscription(sub database.Subscription) bool {
	switch strings.ToLower(strings.TrimSpace(sub.AnimeType)) {
	case "movie", "film", "劇場版", "剧场版":
		return true
	}
	return false
}

// 首个成员沿用占位记录的主键，保留历史引用。按具体来源去重使同集不同版本各自可追踪。
func storeBatchMembers(ctx context.Context, parent database.Episode, members []database.Episode) ([]database.Episode, error) {
	var ready []database.Episode
	err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先获取 SQLite 写锁，再对身份和快照状态做一致性检查。
		if err := tx.Exec("UPDATE episodes SET id = id WHERE 0").Error; err != nil {
			return err
		}
		var count int64
		if err := parent.OrganizationSnapshot(tx).Count(&count).Error; err != nil || count != 1 {
			return err
		}
		reused := false
		for _, m := range members {
			if m.SourceFile != "" && m.FinalPath == "" {
				var existing int64
				hashCondition := "torrent_hash = ?"
				if magnet.IsHexInfoHash(m.TorrentHash) {
					hashCondition += " COLLATE NOCASE"
				}
				if err := tx.Model(&database.Episode{}).Where("id <> ? AND subscription_id = ? AND source_file = ?", parent.ID, m.SubscriptionID, m.SourceFile).
					Where(hashCondition, m.TorrentHash).Count(&existing).Error; err != nil {
					return err
				}
				if existing > 0 {
					continue
				}
			}
			if !reused {
				m.Model = parent.Model
				reused = true
			}
			if err := tx.Save(&m).Error; err != nil {
				return err
			}
			if m.Status != database.EpisodeNeedsReview {
				ready = append(ready, m)
			}
		}
		if !reused {
			return tx.Unscoped().Delete(&parent).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ready, nil
}

var batchNumbers = regexp.MustCompile(`(?i)[\[【]\s*(\d{1,3})\s*[-~～]\s*(\d{1,3})\s*[\]】]`)
var totalNumbers = regexp.MustCompile(`全\s*(\d{1,3})\s*[集话話]`)

// 旧移动模式可能只把合集中的一个视频移到了 E00；仅在能唯一识别时恢复它。
func (s *Scheduler) restoreMovedBatchMember(parent database.Episode, sub database.Subscription, members []database.Episode) []database.Episode {
	original := s.legacyReviewMember(parent, sub, "旧合集已整理文件无法唯一对应缺失的集号，请核对原文件")
	// 旧库原件沿用占位行主键和路径，已核实的其他成员可分别整理。
	candidate := append([]database.Episode{original}, members...)
	if original.ReviewKind == "" {
		return candidate
	}
	start, end := 0, 0
	if m := batchNumbers.FindStringSubmatch(parent.OriginalName); len(m) > 0 {
		start, _ = strconv.Atoi(m[1])
		end, _ = strconv.Atoi(m[2])
	} else if m := totalNumbers.FindStringSubmatch(parent.OriginalName); len(m) > 0 {
		start = 1
		end, _ = strconv.Atoi(m[1])
	}
	if start < 1 || end < start || end > 2000 {
		return candidate
	}
	present := map[int]bool{}
	for _, ep := range members {
		if ep.Status == database.EpisodeNeedsReview {
			return candidate
		}
		if ep.Season == parent.Season && ep.Number == float32(int(ep.Number)) {
			present[int(ep.Number)] = true
		}
	}
	missing := 0
	for n := start; n <= end; n++ {
		if present[n] {
			continue
		}
		if missing != 0 {
			return candidate
		}
		missing = n
	}
	if missing == 0 {
		return candidate
	}
	original.Number = float32(missing)
	original.Title = fmt.Sprintf("%s E%d", parent.Title, missing)
	original.Status, original.ReviewKind, original.ReviewReason = "renaming", "", ""
	candidate[0] = original
	return candidate
}

// 仅修复能从原始种子标题明确识别的旧季号；不删除任何已有媒体文件。
func (s *Scheduler) repairLegacyEpisodes(ctx context.Context) error {
	const marker = "scheduler_episode_identity_v3"
	var done int64
	if err := database.DB.Model(&database.Setting{}).Where("key = ?", marker).Count(&done).Error; err != nil || done > 0 {
		return err
	}
	db := database.DB.WithContext(ctx)
	var subs []database.Subscription
	if err := db.Unscoped().Find(&subs).Error; err != nil {
		return err
	}
	owners := make(map[uint]database.Subscription, len(subs))
	for _, sub := range subs {
		owners[sub.ID] = sub
	}
	var episodes []database.Episode
	if err := db.Model(&database.Setting{}).Where("key = ?", "scheduler_episode_identity_v2").Count(&done).Error; err != nil {
		return err
	}
	query := db.Unscoped().Where("season = 1 OR torrent_hash LIKE 'url:%' OR number = 0")
	if done > 0 {
		query = db.Where("number = 0 AND status = ?", "organized")
	}
	if err := query.Find(&episodes).Error; err != nil {
		return err
	}
	for _, ep := range episodes {
		sub, exists := owners[ep.SubscriptionID]
		if exists && !ep.DeletedAt.Valid && !sub.DeletedAt.Valid && ep.Status == "organized" && ep.FinalPath != "" && unknownLegacyEpisode(ep, sub) {
			if err := s.recoverLegacyEpisode(ctx, ep, sub); err != nil {
				return err
			}
			continue
		}
		raw := ep.OriginalName
		if raw == "" {
			raw = ep.Title
		}
		parsed := parser.ExtractEpisodeDetailed(raw)
		updates := map[string]any{}
		if strings.HasPrefix(ep.TorrentHash, "url:") {
			updates["origin_key"] = ep.TorrentHash
			updates["torrent_hash"] = ""
		}
		if parsed.IsSpecial && parsed.Season == 0 && ep.Season == 1 && parsed.Episode == ep.Number {
			updates["season"] = 0
		}
		if !parsed.ExplicitSeason && !parsed.IsSpecial && parsed.Season == 1 && parsed.Episode > 0 && parsed.Episode == ep.Number && ep.Season == 1 && owners[ep.SubscriptionID].Season > 1 {
			updates["season"] = owners[ep.SubscriptionID].Season
		}
		if target, changed := updates["season"]; changed {
			sub := owners[ep.SubscriptionID]
			var targetPath string
			if ep.Status == "organized" && ep.FinalPath != "" {
				var safe bool
				targetPath, safe = s.legacyRenameDestination(ctx, ep, sub, target.(int))
				if !safe {
					continue
				}
				updates["status"] = "renaming"
			}
			var count int64
			query := db.Unscoped().Model(&database.Episode{}).Where("id <> ?", ep.ID)
			conflict := db.Unscoped().Where("subscription_id = ? AND season = ? AND number = ?", ep.SubscriptionID, target, ep.Number)
			if ep.FinalPath != "" {
				conflict = conflict.Or("final_path = ?", ep.FinalPath)
			}
			if targetPath != "" {
				conflict = conflict.Or("final_path = ?", targetPath)
			}
			if err := query.Where(conflict).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
		}
		if parsed.IsBatch && ep.Number == 0 && ep.Status == "organized" && ep.SourceFile == "" {
			// 保留已整理的旧文件，等待重新核对完整种子目录。
			updates["status"] = "downloaded"
		}
		if len(updates) > 0 {
			if err := db.Unscoped().Model(&ep).Where("updated_at = ?", ep.UpdatedAt).Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	return db.Where("key = ?", marker).FirstOrCreate(&database.Setting{Key: marker, Value: "true"}).Error
}

func (s *Scheduler) legacyRenameDestination(ctx context.Context, ep database.Episode, sub database.Subscription, season int) (string, bool) {
	if !movieSubscription(sub) && !parser.ValidEpisodeIdentity(season, ep.Number) {
		return "", false
	}
	planner, ok := s.organizer.(interface {
		Preview(context.Context, string, core.Anime, core.Episode) (organizer.PreviewResult, error)
	})
	if !ok {
		return "", false
	}
	plan, err := planner.Preview(ctx, ep.FinalPath, organizedAnime(sub), core.Episode{Season: season, Number: ep.Number, Title: ep.Title})
	if err != nil {
		return "", false
	}
	dst, err := os.Lstat(plan.FinalPath)
	if os.IsNotExist(err) {
		return plan.FinalPath, true
	}
	if err != nil {
		return "", false
	}
	if !dst.Mode().IsRegular() {
		return "", false
	}
	equal, err := organizer.SameMediaContents(ctx, ep.FinalPath, plan.FinalPath)
	return plan.FinalPath, err == nil && equal
}

func existingVideo(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Size() > 0 && mediaFile(path)
}
