package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"gorm.io/gorm"
)

// 下载器仅为可确认的任务返回结果；不支持的标识及不完整快照保留为未知。
type taskPresenceChecker interface {
	TaskPresence(context.Context, []string) (map[string]bool, error)
}

func (s *Server) handleSubscriptionDownloadTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的订阅 ID"})
		return
	}
	db := database.DB.WithContext(r.Context())
	var sub database.Subscription
	if err := db.Select("id").First(&sub, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅不存在"})
		} else {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取订阅失败"})
		}
		return
	}

	var refs []string
	if err := db.Model(&database.Episode{}).
		Where("subscription_id = ? AND status IN ? AND torrent_hash <> ''", id, []string{"downloading", "downloaded"}).
		Distinct().Pluck("torrent_hash", &refs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取下载任务标识失败"})
		return
	}
	states := make(map[string]string, len(refs))
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		key := strings.ToLower(strings.TrimSpace(ref))
		if key == "" || strings.HasPrefix(key, "url:") || states[key] != "" {
			continue
		}
		states[key] = "unknown"
		ids = append(ids, key)
	}
	if len(ids) > 0 {
		dl := s.downloader
		if dynamic, ok := dl.(*downloader.DynamicDownloader); ok {
			dl = dynamic.GetCurrent()
		}
		checker, ok := dl.(taskPresenceChecker)
		if !ok {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "当前下载器无法核对任务"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		found, err := checker.TaskPresence(ctx, ids)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "无法从当前下载器核对任务，请检查连接后重试"})
			return
		}
		for _, key := range ids {
			if present, known := found[key]; known {
				if present {
					states[key] = "present"
				} else {
					states[key] = "missing"
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": states})
}
