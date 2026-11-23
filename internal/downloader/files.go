package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// FileSelection 只包含下载器确认已选并下载完整的文件。Paths 均位于任务保存目录内。
type FileSelection struct {
	Files   []CompletedFile
	Partial bool
	// 仅当下载器列出被跳过的文件，且它们都是已知附件时为真。
	onlySidecarsSkipped bool
}

// IsSingleVideo 仅对清单中唯一的视频自动确认。未知的未选文件也可能是另一版本。
func (s FileSelection) IsSingleVideo() bool {
	if s.Partial && !s.onlySidecarsSkipped {
		return false
	}
	count := 0
	for _, f := range s.Files {
		if core.IsVideoFile(f.Path) {
			count++
		} else if !isSidecar(f.Path) {
			return false
		}
	}
	return count == 1
}

func (s *FileSelection) skip(name string) {
	// 之后遇到字幕时，仍须记住前面跳过的视频或未知文件。
	s.onlySidecarsSkipped = (!s.Partial || s.onlySidecarsSkipped) && isSidecar(name)
	s.Partial = true
}

func isSidecar(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".srt", ".ass", ".ssa", ".vtt", ".sub", ".idx", ".sup", ".smi",
		".nfo", ".txt", ".jpg", ".jpeg", ".png", ".webp", ".ttf", ".otf", ".sfv", ".md5", ".sha256":
		return true
	}
	return false
}

type CompletedFile struct {
	ModTime int64 // 调度器在核对下载目录时记录的文件修改时间。
	Path    string
	Size    int64
}
type FileLister interface {
	CompletedFiles(context.Context, core.DownloadTask) (FileSelection, error)
}

func (s *FileSelection) add(root, name string, size int64, complete bool) error {
	if !filepath.IsLocal(name) || root == "" || size < 0 || !complete {
		return fmt.Errorf("下载器尚未确认所选文件完整: %s", name)
	}
	s.Files = append(s.Files, CompletedFile{Path: filepath.Join(root, name), Size: size})
	return nil
}

func (q *QBittorrent) CompletedFiles(ctx context.Context, task core.DownloadTask) (FileSelection, error) {
	var result FileSelection
	if err := q.ensureLogin(ctx); err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.host+"/api/v2/torrents/files?hash="+url.QueryEscape(task.Hash), nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Referer", q.host)
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("qBittorrent 文件清单返回 HTTP %d", resp.StatusCode)
	}
	var files []struct {
		Name     string
		Size     *int64
		Priority *int
		Progress *float64
	}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return result, err
	}
	for _, f := range files {
		if f.Priority == nil || f.Size == nil {
			return result, fmt.Errorf("qBittorrent 文件清单缺少选择状态或大小")
		}
		if *f.Priority == 0 {
			result.skip(f.Name)
			continue
		}
		if err := result.add(task.SavePath, f.Name, *f.Size, *f.Priority > 0 && f.Progress != nil && *f.Progress == 1); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (t *Transmission) CompletedFiles(ctx context.Context, task core.DownloadTask) (FileSelection, error) {
	var result FileSelection
	resp, err := t.call(ctx, "torrent-get", map[string]any{"ids": []string{task.Hash}, "fields": []string{"hashString", "files", "fileStats"}})
	if err != nil {
		return result, err
	}
	var payload struct {
		Torrents []struct {
			Hash  string `json:"hashString"`
			Files []struct {
				Name           string
				Length         *int64
				BytesCompleted *int64
			}
			FileStats []struct{ Wanted *bool }
		}
	}
	if err := json.Unmarshal(resp.Arguments, &payload); err != nil {
		return result, err
	}
	if len(payload.Torrents) != 1 {
		return result, fmt.Errorf("Transmission 未返回指定任务的文件清单")
	}
	torrent := payload.Torrents[0]
	if !strings.EqualFold(torrent.Hash, task.Hash) || len(torrent.Files) != len(torrent.FileStats) {
		return result, fmt.Errorf("Transmission 文件选择信息不一致")
	}
	for i, f := range torrent.Files {
		wanted := torrent.FileStats[i].Wanted
		if wanted == nil || f.Length == nil {
			return result, fmt.Errorf("Transmission 文件选择状态或大小未知")
		}
		if !*wanted {
			result.skip(f.Name)
			continue
		}
		if err := result.add(task.SavePath, f.Name, *f.Length, f.BytesCompleted != nil && *f.Length == *f.BytesCompleted); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (a *Aria2) CompletedFiles(ctx context.Context, task core.DownloadTask) (FileSelection, error) {
	var result FileSelection
	if task.ClientID == "" {
		return result, fmt.Errorf("Aria2 缺少文件所属任务 GID")
	}
	keys := []string{"gid", "status", "totalLength", "completedLength", "files", "bitfield", "numPieces", "pieceLength"}
	raw, err := a.call(ctx, "aria2.tellStatus", []any{task.ClientID, keys})
	if err != nil {
		return result, err
	}
	var manifest aria2FileManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return result, err
	}
	if !strings.EqualFold(manifest.GID, task.ClientID) {
		return result, fmt.Errorf("Aria2 未返回指定任务的文件清单")
	}
	archived := manifest.confirmsSelectedBytes()
	for _, f := range manifest.Files {
		if f.Selected == nil || f.Length == nil {
			return result, fmt.Errorf("Aria2 文件选择状态或大小未知")
		}
		if !*f.Selected {
			result.skip(f.Path)
			continue
		}
		name, err := filepath.Rel(task.SavePath, f.Path)
		if err != nil {
			return result, err
		}
		complete := f.Completed != nil && (*f.Completed == *f.Length || archived && *f.Completed == 0)
		if err := result.add(task.SavePath, name, *f.Length, complete); err != nil {
			return result, err
		}
	}
	return result, nil
}
