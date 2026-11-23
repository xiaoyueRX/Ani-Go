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
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
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

// 先确认整个合集的文件与季集能一一对应，再用同一事务将占位剧集换成文件级记录。
func (s *Scheduler) expandBatch(ctx context.Context, parent database.Episode, sub database.Subscription, dir string, selection *downloader.FileSelection) ([]database.Episode, error) {
	var members []database.Episode
	seen := map[string]struct{}{}
	visit := func(path string, size, modTime int64) error {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		parsed := parser.ExtractEpisodeDetailed(title)
		if parsed.Episode <= 0 {
			n, err := strconv.Atoi(title)
			if err != nil || n < 1 || n > 2000 {
				return fmt.Errorf("无法确定文件集号: %s", rel)
			}
			title = fmt.Sprintf("E%d", n)
		}
		// 文件未注明季号时，继承合集标题的季数。
		owner := sub
		owner.Season = parent.Season
		season, num := s.resolveSubscriptionEpisode(ctx, core.TorrentItem{Title: title}, owner)
		if num <= 0 {
			return fmt.Errorf("无法确定文件集号: %s", rel)
		}
		key := fmt.Sprintf("%d/%d/%g", sub.ID, season, num)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("多文件对应同一集 %s", rel)
		}
		seen[key] = struct{}{}
		member := parent
		member.Model = gorm.Model{}
		member.ReservationKey = &key
		member.SourceFile = rel
		member.DownloadRoot = dir
		member.Season = season
		member.Number = num
		member.Title = title
		member.Status = "downloaded"
		member.OrganizedAt = nil
		member.FinalPath = ""
		member.FileSize = size
		member.SourceModTime = modTime
		members = append(members, member)
		return nil
	}
	if selection != nil {
		for _, f := range selection.Files {
			if err := visit(f.Path, f.Size, f.ModTime); err != nil {
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
			if info.Size() <= 0 {
				return fmt.Errorf("视频文件为空: %s", path)
			}
			return visit(path, info.Size(), 0)
		})
		if err != nil {
			return nil, err
		}
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("合集目录中未找到视频: %s", dir)
	}
	if selection == nil || !selection.Partial || parent.FinalPath != "" {
		if err := restoreMovedBatchMember(parent, &members); err != nil {
			return nil, err
		}
	}
	var created []database.Episode
	err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current database.Episode
		if err := tx.Where("id = ? AND status = ?", parent.ID, "downloaded").First(&current).Error; err != nil {
			return err
		}
		for _, m := range members {
			var existing int64
			if err := tx.Model(&database.Episode{}).Where("subscription_id = ? AND season = ? AND number = ?", sub.ID, m.Season, m.Number).Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				continue
			}
			if err := tx.Create(&m).Error; err != nil {
				return err
			}
			created = append(created, m)
		}
		return tx.Unscoped().Delete(&current).Error
	})
	return created, err
}

var batchNumbers = regexp.MustCompile(`(?i)[\[【]\s*(\d{1,3})\s*[-~～]\s*(\d{1,3})\s*[\]】]`)
var totalNumbers = regexp.MustCompile(`全\s*(\d{1,3})\s*[集话話]`)

// 旧移动模式可能只把合集中的一个视频移到了 E00；仅在能唯一识别时恢复它。
func restoreMovedBatchMember(parent database.Episode, members *[]database.Episode) error {
	start, end := 0, 0
	if m := batchNumbers.FindStringSubmatch(parent.OriginalName); len(m) > 0 {
		start, _ = strconv.Atoi(m[1])
		end, _ = strconv.Atoi(m[2])
	} else if m := totalNumbers.FindStringSubmatch(parent.OriginalName); len(m) > 0 {
		start = 1
		end, _ = strconv.Atoi(m[1])
	}
	if start < 1 || end < start || end > 2000 {
		return nil
	}
	present := map[int]bool{}
	for _, ep := range *members {
		if ep.Season == parent.Season && ep.Number == float32(int(ep.Number)) {
			present[int(ep.Number)] = true
		}
	}
	var missing []int
	for n := start; n <= end; n++ {
		if !present[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if len(missing) != 1 || parent.FinalPath == "" || !existingVideo(parent.FinalPath) {
		return fmt.Errorf("合集缺少可确认的文件，缺失集号 %v", missing)
	}
	n := missing[0]
	ep := parent
	ep.Model = gorm.Model{}
	ep.Number = float32(n)
	ep.Title = fmt.Sprintf("%s E%d", parent.Title, n)
	ep.Status = "renaming"
	key := fmt.Sprintf("%d/%d/%d", parent.SubscriptionID, parent.Season, n)
	ep.ReservationKey = &key
	*members = append(*members, ep)
	return nil
}

// 仅修复能从原始种子标题明确识别的旧季号；不删除任何已有媒体文件。
func (s *Scheduler) repairLegacyEpisodes(ctx context.Context) error {
	const marker = "scheduler_episode_identity_v2"
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
	if err := db.Unscoped().Where("season = 1 OR torrent_hash LIKE 'url:%' OR number = 0").Find(&episodes).Error; err != nil {
		return err
	}
	for _, ep := range episodes {
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
