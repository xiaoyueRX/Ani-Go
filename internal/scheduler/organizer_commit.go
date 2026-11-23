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
		"UPDATE episodes SET status = ?, final_path = ?, organized_at = ?, updated_at = ?, move_source_path = ?, move_source_mod_time = ? WHERE id = ? AND status = ? AND deleted_at IS NULL",
		"organized", dst, now, now, move.Path, move.ModTime, ep.ID, ep.Status)
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
	commit := func(path string, move *organizer.MoveSource) error {
		if move != nil {
			return durableMoveCommit(ctx, ep, path, *move)
		}
		result := db.Model(&database.Episode{}).Where("id = ? AND status = ?", ep.ID, ep.Status).Updates(map[string]any{"status": "organized", "final_path": path, "organized_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("剧集 %d 的整理状态已变更", ep.ID)
		}
		return nil
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

func finishPendingMoves(ctx context.Context) {
	var pending []database.Episode
	db := database.DB.WithContext(ctx)
	if err := db.Where("status = ? AND move_source_path <> ''", "organized").Find(&pending).Error; err != nil {
		log.Printf("读取待清理移动源失败: %v", err)
		return
	}
	for _, ep := range pending {
		move := organizer.MoveSource{Path: ep.MoveSourcePath, ModTime: ep.MoveSourceModTime}
		if err := move.Cleanup(ctx, ep.FinalPath); err != nil {
			log.Printf("保留待清理移动源 [%d]: %v", ep.ID, err)
			continue
		}
		if err := db.Model(&database.Episode{}).Where("id = ? AND move_source_path = ?", ep.ID, move.Path).Updates(map[string]any{"move_source_path": "", "move_source_mod_time": 0}).Error; err != nil {
			log.Printf("更新移动源清理状态失败 [%d]: %v", ep.ID, err)
		}
	}
}
