package scheduler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
)

// 仅在下载器已确认完成、且本地证据明确矛盾时进入人工复核。
var errOrganizationReview = errors.New("整理需要复核")

type verifiedSelection struct {
	downloader.FileSelection
	issues map[string]string
}

func availableVideos(root string, selected downloader.FileSelection, legacyMoved bool) (verifiedSelection, error) {
	videos := verifiedSelection{FileSelection: downloader.FileSelection{Partial: selected.Partial}, issues: make(map[string]string)}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return videos, err
	}
	for _, f := range selected.Files {
		if !mediaFile(f.Path) {
			continue
		}
		rel, err := filepath.Rel(root, f.Path)
		if err != nil || !filepath.IsLocal(rel) {
			return videos, fmt.Errorf("%w: 下载器报告的视频超出任务目录", errOrganizationReview)
		}
		f.ModTime = 0
		stat, err := os.Lstat(f.Path)
		if os.IsNotExist(err) {
			if !legacyMoved {
				videos.issues[f.Path] = "下载器已确认完成，但本地缺少该视频，请核对下载目录"
				videos.Files = append(videos.Files, f)
			}
			continue
		}
		if err != nil {
			return videos, err
		}
		if !stat.Mode().IsRegular() || stat.Size() != f.Size || f.Size <= 0 {
			videos.issues[f.Path] = fmt.Sprintf("源文件必须是非空普通文件，下载器记录 %d 字节，本地为 %d 字节（%s）", f.Size, stat.Size(), stat.Mode().Type())
		} else {
			ok, err := sourceWithinRoot(base, f.Path)
			if err != nil {
				return videos, err
			}
			if !ok {
				videos.issues[f.Path] = "源文件实际路径在任务下载目录之外"
			} else {
				f.ModTime = stat.ModTime().UnixNano()
			}
		}
		videos.Files = append(videos.Files, f)
	}
	if len(videos.Files) == 0 {
		return videos, fmt.Errorf("%w: 任务未选中可整理的完整视频", errOrganizationReview)
	}
	return videos, nil
}

func sourceWithinRoot(resolvedRoot, path string) (bool, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	return filepath.IsLocal(rel), err
}

// 老版合集记录缺少下载器逐文件确认。只有已确认且之后未变化的源可离线重试。
func verifiedBatchSource(ep database.Episode, dl core.Downloader) bool {
	if ep.DownloadRoot == "" || !filepath.IsLocal(ep.SourceFile) {
		return false
	}
	path := filepath.Join(ep.DownloadRoot, ep.SourceFile)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() != ep.FileSize || !mediaFile(path) {
		return false
	}
	base, err := filepath.EvalSymlinks(ep.DownloadRoot)
	if err != nil {
		return false
	}
	inside, err := sourceWithinRoot(base, path)
	if err != nil || !inside {
		return false
	}
	if ep.SourceModTime != 0 {
		return ep.SourceModTime == info.ModTime().UnixNano()
	}
	_, canVerify := dl.(downloader.FileLister)
	return dl != nil && !canVerify
}
