package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

// SQLite 的 NORMAL/WAL 模式允许掉电时回滚最近的提交。源文件清理必须晚于一次持久提交。
func durableMoveCommit(ctx context.Context, ep database.Episode, dst string, move organizer.MoveSource) error {
	pool, err := database.DB.DB()
	if err != nil {
		return err
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var previous int
	if err := conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&previous); err != nil {
		return err
	}
	if previous < 2 {
		if _, err := conn.ExecContext(ctx, "PRAGMA synchronous=FULL"); err != nil {
			return err
		}
		defer func() {
			if _, err := conn.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("PRAGMA synchronous=%d", previous)); err != nil {
				log.Printf("恢复 SQLite 同步级别失败: %v", err)
			}
		}()
	}
	now := time.Now()
	result, err := conn.ExecContext(ctx,
		"UPDATE episodes SET status = ?, final_path = ?, organized_at = ?, updated_at = ?, move_source_path = ?, move_source_mod_time = ?, move_volume_root = ?, move_volume_id = ?, movie_feature = ? WHERE id = ? AND status = ? AND (updated_at = ? OR (updated_at IS NULL AND ?)) AND deleted_at IS NULL",
		"organized", dst, now, now, move.Path, move.ModTime, move.Volume.Root, move.Volume.ID, ep.MovieFeature, ep.ID, ep.Status, ep.UpdatedAt, ep.UpdatedAt.IsZero())
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return fmt.Errorf("剧集 %d 的整理状态已变更", ep.ID)
	}
	return err
}

func (s *Scheduler) commitOrganization(ctx context.Context, ep database.Episode, source string, anime core.Anime, coreEp core.Episode) (string, error) {
	db := database.DB.WithContext(ctx)
	var matches int64
	if err := ep.OrganizationSnapshot(db).Count(&matches).Error; err != nil {
		return "", err
	}
	if matches != 1 {
		return "", fmt.Errorf("剧集 %d 的整理快照已失效", ep.ID)
	}
	// 已下载电影只有通过整份清单或人工确认的来源才能走到此处；与最终路径一起提交正片身份。
	if ep.Status == "downloaded" && core.TMDBMediaType(anime.Type) == "movie" && ep.HasBoundReviewFile() {
		feature := true
		ep.MovieFeature = &feature
	}
	commit := func(path string, move *organizer.MoveSource) error {
		if move != nil {
			return durableMoveCommit(ctx, ep, path, *move)
		}
		result := ep.OrganizationSnapshot(db).Updates(map[string]any{"status": "organized", "final_path": path, "organized_at": time.Now(), "movie_feature": ep.MovieFeature})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("剧集 %d 的整理状态已变更", ep.ID)
		}
		return nil
	}
	if org, ok := s.organizer.(interface {
		OrganizeVersionAndCommit(context.Context, string, core.Anime, core.Episode, string, func(string, *organizer.MoveSource) error) (string, error)
	}); ok {
		// 历史单集记录未存成员名时，用实际路径与下载任务一起标识来源，重启后仍可复用。
		member := ep.SourceFile
		if member == "" {
			member = source
		}
		key := database.EpisodeFileReservationKey(ep.SubscriptionID, ep.TorrentHash, member)
		return org.OrganizeVersionAndCommit(ctx, source, anime, coreEp, key, commit)
	}
	if org, ok := s.organizer.(interface {
		OrganizeAndCommit(context.Context, string, core.Anime, core.Episode, func(string, *organizer.MoveSource) error) (string, error)
	}); ok {
		return org.OrganizeAndCommit(ctx, source, anime, coreEp, commit)
	}
	path, err := s.organizer.Organize(ctx, source, anime, coreEp)
	if err != nil {
		return "", err
	}
	return path, commit(path, nil)
}

func (s *Scheduler) finishPendingMoves(ctx context.Context) {
	var pending []database.Episode
	db := database.DB.WithContext(ctx)
	if err := db.Where("status = ? AND move_source_path <> ''", "organized").Find(&pending).Error; err != nil {
		log.Printf("读取待清理移动源失败: %v", err)
		return
	}
	for _, ep := range pending {
		volume := organizer.LibraryVolume{Root: ep.MoveVolumeRoot, ID: ep.MoveVolumeID}
		if volume.ID == "" {
			if org, ok := s.organizer.(interface {
				PendingMoveVolume(context.Context, database.Episode) (organizer.LibraryVolume, error)
			}); ok {
				var err error
				volume, err = org.PendingMoveVolume(ctx, ep)
				if err != nil {
					log.Printf("保留待清理移动源 [%d]: %v", ep.ID, err)
					continue
				}
			}
		}
		move := organizer.MoveSource{Path: ep.MoveSourcePath, ModTime: ep.MoveSourceModTime, Volume: volume}
		if err := move.Cleanup(ctx, ep.FinalPath); err != nil {
			log.Printf("保留待清理移动源 [%d]: %v", ep.ID, err)
			continue
		}
		if err := db.Model(&database.Episode{}).Where("id = ? AND move_source_path = ?", ep.ID, move.Path).Updates(map[string]any{"move_source_path": "", "move_source_mod_time": 0, "move_volume_root": "", "move_volume_id": ""}).Error; err != nil {
			log.Printf("更新移动源清理状态失败 [%d]: %v", ep.ID, err)
		}
	}
}
