package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

// 已完成集数与剧集状态在同一个事务中更新，供时间表和调度器使用。
func updateEpisodeProgress(tx *gorm.DB, subID uint) error {
	_, err := database.SyncEpisodeProgress(tx, subID, false)
	return err
}

type deleteFailure struct {
	status  int
	message string
}

func (f *deleteFailure) write(w http.ResponseWriter) {
	writeJSON(w, f.status, errorResponse{Error: f.message})
}

// 同一任务可能对应多个剧集或订阅。回收站中的记录仍可恢复，也持有文件。
func (s *Server) deleteOwnedFiles(ctx context.Context, episodes []database.Episode, removeRecords func([]uint) error) *deleteFailure {
	if len(episodes) == 0 {
		return nil
	}
	if s.downloader == nil {
		return &deleteFailure{http.StatusConflict, "下载器不可用，请选择仅删记录"}
	}
	byHash := make(map[string][]uint)
	hashes := make([]string, 0)
	ids := make([]uint, 0, len(episodes))
	for _, ep := range episodes {
		hash := strings.ToLower(strings.TrimSpace(ep.TorrentHash))
		if hash == "" || strings.HasPrefix(hash, "url:") {
			return &deleteFailure{http.StatusConflict, "无法定位真实下载任务，请选择仅删记录"}
		}
		if _, seen := byHash[hash]; !seen {
			hashes = append(hashes, hash)
		}
		byHash[hash] = append(byHash[hash], ep.ID)
		ids = append(ids, ep.ID)
	}
	var shared int64
	if err := database.DB.Unscoped().Model(&database.Episode{}).
		Where("lower(trim(torrent_hash)) IN ? AND id NOT IN ?", hashes, ids).Count(&shared).Error; err != nil {
		return &deleteFailure{http.StatusInternalServerError, "检查下载任务归属失败"}
	}
	if shared > 0 {
		return &deleteFailure{http.StatusConflict, "其他剧集或回收站记录仍使用所选下载任务，请选择仅删记录"}
	}
	// 每个任务只提交一次。逐任务记录成功结果，部分失败可在刷新后重试。
	for _, hash := range hashes {
		if err := s.downloader.Delete(ctx, hash, true); err != nil {
			return &deleteFailure{http.StatusBadGateway, "下载器删除失败，请刷新后重试"}
		}
		if err := removeRecords(byHash[hash]); err != nil {
			return &deleteFailure{http.StatusInternalServerError, "更新剧集记录失败"}
		}
	}
	return nil
}

// 剧集删除与回收站中的整部订阅删除各自独立，恢复订阅时不会找回用户已移除的剧集。
func (s *Server) handleBatchDeleteEpisodes(w http.ResponseWriter, r *http.Request) {
	subID, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	var req batchDeleteRequest
	if err != nil || subID == 0 || json.NewDecoder(r.Body).Decode(&req) != nil || len(req.IDs) == 0 || len(req.IDs) > 1000 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请提供有效订阅及 1 至 1000 个剧集 ID"})
		return
	}
	for _, episodeID := range req.IDs {
		if episodeID == 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "剧集 ID 必须为正数"})
			return
		}
	}
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	var sub database.Subscription
	if err = database.DB.First(&sub, subID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅不存在"})
		} else {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取订阅失败"})
		}
		return
	}
	var episodes []database.Episode
	if err = database.DB.Where("id IN ?", req.IDs).Find(&episodes).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取剧集失败"})
		return
	}
	// 先校验整批，请求混入其他订阅时不能产生任何删除副作用。
	for _, ep := range episodes {
		if ep.SubscriptionID != sub.ID {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "所选剧集不属于当前订阅"})
			return
		}
	}
	deleted := int64(0)
	remove := func(ids []uint) error {
		return database.DB.Transaction(func(tx *gorm.DB) error {
			res := tx.Unscoped().Where("subscription_id = ? AND id IN ?", sub.ID, ids).Delete(&database.Episode{})
			if res.Error != nil {
				return res.Error
			}
			deleted += res.RowsAffected
			return updateEpisodeProgress(tx, sub.ID)
		})
	}
	if req.DeleteFiles {
		if failure := s.deleteOwnedFiles(r.Context(), episodes, remove); failure != nil {
			failure.write(w)
			return
		}
	} else if err := remove(req.IDs); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "删除剧集记录失败"})
		return
	}
	writeJSON(w, http.StatusOK, batchDeleteResponse{Deleted: int(deleted)})
}

// 单条与批量订阅删除使用同一套归属检查和事务。
func (s *Server) deleteSubscriptions(ctx context.Context, ids []uint, deleteFiles bool) (int, *deleteFailure) {
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	if deleteFiles {
		var episodes []database.Episode
		if err := database.DB.Where("subscription_id IN ?", ids).Find(&episodes).Error; err != nil {
			return 0, &deleteFailure{http.StatusInternalServerError, "读取剧集记录失败"}
		}
		remove := func(episodeIDs []uint) error {
			return database.DB.Where("subscription_id IN ? AND id IN ?", ids, episodeIDs).Delete(&database.Episode{}).Error
		}
		if failure := s.deleteOwnedFiles(ctx, episodes, remove); failure != nil {
			return 0, failure
		}
	}
	var affected int64
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subscription_id IN ?", ids).Delete(&database.Episode{}).Error; err != nil {
			return err
		}
		res := tx.Where("id IN ?", ids).Delete(&database.Subscription{})
		affected = res.RowsAffected
		return res.Error
	})
	if err != nil {
		return 0, &deleteFailure{http.StatusInternalServerError, "删除订阅失败"}
	}
	return int(affected), nil
}
