package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
	"gorm.io/gorm"
)

type reviewConfirmation struct {
	Season       *int     `json:"season"`
	Number       *float32 `json:"number"`
	ConfirmMovie *bool    `json:"confirm_movie"`
}

// 空请求体只重新核验文件；赋号和电影正片确认是两种独立的意图。
func parseReviewConfirmation(body io.Reader) (reviewConfirmation, string, bool) {
	var input *reviewConfirmation
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&input)
	if err == io.EOF {
		return reviewConfirmation{}, "", true
	}
	if err != nil || input == nil || dec.Decode(new(json.RawMessage)) != io.EOF {
		return reviewConfirmation{}, "", false
	}
	if input.ConfirmMovie != nil {
		return *input, database.ReviewMovieSelection, *input.ConfirmMovie && input.Season == nil && input.Number == nil
	}
	ok := input.Season != nil && input.Number != nil && parser.ValidEpisodeIdentity(*input.Season, *input.Number)
	return *input, database.ReviewEpisodeIdentity, ok
}

func currentReviewSource(ep database.Episode) bool {
	root, err := filepath.EvalSymlinks(ep.DownloadRoot)
	if err != nil {
		return false
	}
	path := filepath.Join(root, ep.SourceFile)
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || !filepath.IsLocal(rel) {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() == ep.FileSize && info.ModTime().UnixNano() == ep.SourceModTime
}

func (s *Server) handleRetryOrganization(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, min(strconv.IntSize, 63))
	if err != nil || id == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的剧集 ID"})
		return
	}
	input, kind, ok := parseReviewConfirmation(r.Body)
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请提供有效的季集或正片确认，普通重试请留空请求体"})
		return
	}
	db := database.DB.WithContext(r.Context())
	var ep database.Episode
	err = db.First(&ep, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "剧集不存在"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取复核记录失败"})
		return
	}
	if ep.Status != database.EpisodeNeedsReview {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "记录已不处于复核状态"})
		return
	}
	recheck := kind == "" && ep.ReviewKind != "" && ep.CanRetryOrganization()
	if ep.ReviewKind != kind && !recheck {
		message := "此记录只能重新校验文件"
		switch ep.ReviewKind {
		case database.ReviewEpisodeIdentity:
			message = "请核对源文件后指定季号和集号"
		case database.ReviewMovieSelection:
			message = "请单独确认该源文件是电影正片"
		}
		writeJSON(w, http.StatusConflict, errorResponse{Error: message})
		return
	}

	update := ep.OrganizationSnapshot(db)
	updates := map[string]any{"status": "downloaded", "review_reason": "", "review_kind": ""}
	if recheck {
		// 重读下载器与本地文件后仍须人工确认，禁止沿用旧季集或误认电影花絮。
		updates["review_kind"] = ep.ReviewKind
		updates["confirmed_identity"] = ""
		updates["number"] = 0
	} else if kind != "" {
		if ep.ConfirmationSignature(kind) == "" || !currentReviewSource(ep) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "源文件证据不足或已变化，请重新核验文件"})
			return
		}
		if kind == database.ReviewEpisodeIdentity {
			ep.Season, ep.Number = *input.Season, *input.Number
			updates["season"] = ep.Season
		} else {
			ep.Number = 0
		}
		updates["number"] = ep.Number
		updates["confirmed_identity"] = ep.ConfirmationSignature(kind)
		// 依据具体来源而非相同季集预留，保留多个发行版本。
		other := db.Model(&database.Episode{}).Select("1").Where("id <> ?", ep.ID)
		if ep.TorrentHash != "" {
			key := database.EpisodeFileReservationKey(ep.SubscriptionID, ep.TorrentHash, ep.SourceFile)
			other = other.Where("reservation_key = ?", key)
			updates["reservation_key"] = key
		} else {
			// 历史库内原件不能与其他（含可恢复的）记录共用，也不占用普通下载任务的文件键。
			other = other.Unscoped().Where("final_path = ?", ep.FinalPath)
			updates["reservation_key"] = nil
		}
		update = update.Where("NOT EXISTS (?)", other)
	}
	res := update.Updates(updates)
	switch {
	case res.Error != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "无法重新安排整理"})
	case res.RowsAffected != 1:
		writeJSON(w, http.StatusConflict, errorResponse{Error: "记录已变化，或该源文件已被其他记录占用"})
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "downloaded"})
	}
}
