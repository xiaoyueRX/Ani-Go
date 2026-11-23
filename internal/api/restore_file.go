package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/magnet"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
)

var (
	errRepairUnavailable = errors.New("媒体库未配置")
	errRepairSource      = errors.New("无法确认原下载任务与已完成的源文件，请核对下载器和原件")
	errRepairChanged     = errors.New("记录已变化，请刷新后重新核对库文件")
	errRepairOtherDir    = errors.New("无法核验另一条同名记录的目录，请检查媒体卷和访问权限后重试")
	errRepairShared      = errors.New("其他剧集或回收站记录也使用该目标路径，请手工核对")
	errRepairUnmarked    = errors.New("此媒体库未配置卷标，请确认已挂载正确的媒体卷")
	errRepairTarget      = errors.New("未能补回文件，请检查媒体卷、原件和目标路径")
)

// 恢复只接受原记录绑定的文件；当前下载器必须也将它列为唯一匹配的已完成成员。
func repairSource(ctx context.Context, dl core.Downloader, ep database.Episode, animeType string) (string, error) {
	member := filepath.FromSlash(ep.SourceFile)
	if ep.Status != "organized" || ep.ReviewKind != "" || ep.ReviewReason != "" || ep.MoveSourcePath != "" || ep.FinalPath == "" || ep.TorrentHash == "" || strings.HasPrefix(ep.TorrentHash, "url:") ||
		!filepath.IsAbs(ep.DownloadRoot) || !ep.HasBoundReviewFile() || filepath.Clean(member) != member ||
		!core.IsVideoFile(ep.SourceFile) || ep.SourceModTime == 0 || ep.FileSize <= 0 || !ep.MatchesFileReservation(filepath.ToSlash(ep.SourceFile)) {
		return "", errRepairSource
	}
	if ep.ConfirmedIdentity != "" {
		kind := database.ReviewEpisodeIdentity
		if core.TMDBMediaType(animeType) == "movie" {
			kind = database.ReviewMovieSelection
		}
		if !ep.HasConfirmedIdentity(kind, ep.FileSize, ep.SourceModTime) {
			return "", errRepairSource
		}
	}
	if dynamic, ok := dl.(*downloader.DynamicDownloader); ok {
		dl = dynamic.GetCurrent()
	}
	lister, ok := dl.(downloader.FileLister)
	if !ok {
		return "", errRepairSource
	}
	task, err := dl.GetStatus(ctx, ep.TorrentHash)
	id := magnet.CanonicalTaskID(ep.TorrentHash)
	if err != nil || !task.IsComplete() || (magnet.CanonicalTaskID(task.Hash) != id && magnet.CanonicalTaskID(task.ClientID) != id) || !filepath.IsAbs(task.SavePath) {
		return "", errRepairSource
	}
	source := filepath.Join(ep.DownloadRoot, ep.SourceFile)
	rel, err := filepath.Rel(task.SavePath, source)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return "", errRepairSource
	}
	selected, err := lister.CompletedFiles(ctx, task)
	if err != nil {
		return "", errRepairSource
	}
	matches := 0
	for _, file := range selected.Files {
		if relative, err := filepath.Rel(source, file.Path); err == nil && relative == "." {
			if file.Size != ep.FileSize {
				return "", errRepairSource
			}
			matches++
		}
	}
	// 部分旧记录以任务内容子目录为根，须核对成员也确实处在当前下载器的保存目录内。
	ep.DownloadRoot, ep.SourceFile = task.SavePath, rel
	if matches != 1 || !currentReviewSource(ep) {
		return "", errRepairSource
	}
	return source, nil
}

// 旧记录可能保存相对路径或通过另一入口引用同一父目录。回收站记录也保留归属。
func checkRepairOwner(tx *gorm.DB, ep database.Episode) error {
	target, err := filepath.Abs(ep.FinalPath)
	if err != nil {
		return errRepairTarget
	}
	parent, err := os.Stat(filepath.Dir(target))
	if err != nil {
		return errRepairTarget
	}
	targetName := norm.NFC.String(filepath.Base(target))
	rows, err := tx.Unscoped().Model(&database.Episode{}).Select("final_path").Where("id <> ? AND final_path <> ''", ep.ID).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var other string
		if err := rows.Scan(&other); err != nil {
			return err
		}
		if !strings.EqualFold(norm.NFC.String(filepath.Base(other)), targetName) {
			continue
		}
		absolute, err := filepath.Abs(other)
		if err != nil {
			return errRepairOtherDir
		}
		if filepath.Dir(absolute) == filepath.Dir(target) {
			return errRepairShared
		}
		dir, err := os.Stat(filepath.Dir(absolute))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return errRepairOtherDir
		}
		if os.SameFile(parent, dir) {
			return errRepairShared
		}
	}
	return rows.Err()
}

func (s *Server) restoreEpisodeFile(ctx context.Context, sid, eid uint64, confirmUnmarked bool) error {
	db := database.DB.WithContext(ctx)
	var sub database.Subscription
	if err := db.Select("id", "anime_type").First(&sub, sid).Error; err != nil {
		return err
	}
	var ep database.Episode
	if err := db.Where("subscription_id = ?", sid).First(&ep, eid).Error; err != nil {
		return err
	}
	if s.organizer == nil {
		return errRepairUnavailable
	}
	volume, err := s.organizer.RestoreVolume(sub.AnimeType, db)
	if err != nil {
		return err
	}
	if volume.ID == "" && !confirmUnmarked {
		return errRepairUnmarked
	}
	// 确认只适用于这次请求所见的库根。下载器可能较慢，期间保留目录句柄。
	root, err := os.OpenRoot(volume.Root)
	if err != nil {
		return errRepairTarget
	}
	defer root.Close()
	expected, err := root.Stat(".")
	if err != nil {
		return errRepairTarget
	}

	source, err := repairSource(ctx, s.downloader, ep, sub.AnimeType)
	if err != nil {
		return err
	}
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	return db.Transaction(func(tx *gorm.DB) error {
		// 先取得 SQLite 写锁，避免其他连接在检查归属与建链接之间更改记录。
		if err := tx.Exec("UPDATE episodes SET id = id WHERE 0").Error; err != nil {
			return err
		}
		var current database.Subscription
		if err := tx.Select("id", "anime_type").First(&current, sid).Error; err != nil {
			return err
		}
		effective, err := s.organizer.RestoreVolume(current.AnimeType, tx)
		if err != nil {
			return err
		}
		if sub.AnimeType != current.AnimeType || volume != effective {
			return errRepairChanged
		}
		// 同一条记录的来源、人工身份、季集或状态有过修改时须重新确认。
		var latest database.Episode
		if err := tx.Where("subscription_id = ?", sid).First(&latest, eid).Error; err != nil {
			return err
		}
		if !reflect.DeepEqual(ep, latest) {
			return errRepairChanged
		}
		if err := checkRepairOwner(tx, ep); err != nil {
			return err
		}
		if err := volume.RestoreLibraryLink(ctx, ep, source, expected); err != nil {
			if errors.Is(err, organizer.ErrLinkUnavailable) {
				return organizer.ErrLinkUnavailable
			}
			return errRepairTarget
		}
		return nil
	})
}

func (s *Server) handleRestoreFile(w http.ResponseWriter, r *http.Request) {
	sid, e1 := strconv.ParseUint(r.PathValue("id"), 10, min(strconv.IntSize, 63))
	eid, e2 := strconv.ParseUint(r.PathValue("episodeID"), 10, min(strconv.IntSize, 63))
	var req *struct {
		ConfirmUnmarked json.RawMessage `json:"confirm_unmarked_volume"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	dec.DisallowUnknownFields()
	if e1 != nil || e2 != nil || sid == 0 || eid == 0 || dec.Decode(&req) != nil || req == nil || dec.Decode(new(any)) != io.EOF {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的补回请求"})
		return
	}
	confirmation := string(req.ConfirmUnmarked)
	if confirmation != "" && confirmation != "false" && confirmation != "true" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "卷确认须为布尔值"})
		return
	}
	err := s.restoreEpisodeFile(r.Context(), sid, eid, confirmation == "true")
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"restored": true})
	case errors.Is(err, gorm.ErrRecordNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅或剧集不存在"})
	case err == errRepairUnavailable:
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
	case err == organizer.ErrLinkUnavailable:
		writeJSON(w, http.StatusConflict, errorResponse{Error: "无法建立硬链接，跨卷或文件系统不支持时请手工复制原件"})
	case err == errRepairSource || err == errRepairChanged || err == errRepairShared || err == errRepairOtherDir || err == errRepairUnmarked || err == errRepairTarget:
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取或核对补回记录失败"})
	}
}
