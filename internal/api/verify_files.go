package api

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
	"gorm.io/gorm"
)

// handleVerifyFiles 在任何文件查询前一次性核对所选记录归属，结果按请求中的顺序返回。
func (s *Server) handleVerifyFiles(w http.ResponseWriter, r *http.Request) {
	subID, err := strconv.ParseUint(r.PathValue("id"), 10, 63)
	var input struct {
		IDs []uint `json:"ids"`
	}
	decode := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	decode.DisallowUnknownFields()
	if err != nil || subID == 0 || decode.Decode(&input) != nil || decode.Decode(new(any)) != io.EOF || len(input.IDs) == 0 || len(input.IDs) > 100 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请选择 1 至 100 条本订阅的剧集记录"})
		return
	}
	seen := make(map[uint]bool, len(input.IDs))
	ids := make([]uint, 0, len(input.IDs))
	for _, id := range input.IDs {
		if id == 0 || uint64(id) > math.MaxInt64 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的剧集 ID"})
			return
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	db := database.DB.WithContext(r.Context())
	var sub database.Subscription
	var rows []database.Episode
	err = db.Select("id", "anime_type").First(&sub, subID).Error
	if err == nil {
		err = db.Select("id", "subscription_id", "status", "final_path", "file_size", "source_file", "source_mod_time", "torrent_hash", "reservation_key").
			Where("subscription_id = ? AND id IN ?", subID, ids).Find(&rows).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && len(rows) != len(ids)) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅或所选记录不存在"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取剧集记录失败"})
		return
	}
	if s.organizer == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "媒体库未配置"})
		return
	}
	byID := make(map[uint]database.Episode, len(rows))
	for _, ep := range rows {
		byID[ep.ID] = ep
	}
	for i, id := range ids {
		rows[i] = byID[id]
	}
	results, guarded, err := s.organizer.VerifyLibraryFiles(r.Context(), sub.AnimeType, rows)
	if err != nil {
		writeJSON(w, http.StatusRequestTimeout, errorResponse{Error: "核对已取消"})
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Results       []organizer.LibraryFileCheck `json:"results"`
		VolumeGuarded bool                         `json:"volume_guarded"`
	}{results, guarded})
}
