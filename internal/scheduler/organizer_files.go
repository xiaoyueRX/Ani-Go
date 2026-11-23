package scheduler

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
)

func availableVideos(root string, selected downloader.FileSelection, legacyMoved bool) (downloader.FileSelection, error) {
	videos := downloader.FileSelection{Partial: selected.Partial}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return videos, err
	}
	for _, f := range selected.Files {
		if !mediaFile(f.Path) {
			continue
		}
		stat, err := os.Lstat(f.Path)
		if legacyMoved && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return videos, err
		}
		if !stat.Mode().IsRegular() || stat.Size() != f.Size || f.Size <= 0 {
			return videos, fmt.Errorf("本地视频大小与下载器不符: %s", f.Path)
		}
		resolved, err := filepath.EvalSymlinks(f.Path)
		if err != nil {
			return videos, err
		}
		rel, err := filepath.Rel(base, resolved)
		if err != nil || !filepath.IsLocal(rel) {
			return videos, fmt.Errorf("视频不在任务下载目录内: %s", f.Path)
		}
		f.ModTime = stat.ModTime().UnixNano()
		videos.Files = append(videos.Files, f)
	}
	if len(videos.Files) == 0 {
		return videos, fmt.Errorf("任务未选中可整理的完整视频")
	}
	return videos, nil
}

func selectedVideo(files downloader.FileSelection, content, member, root string) string {
	var largest downloader.CompletedFile
	for _, f := range files.Files {
		if member != "" {
			if filepath.IsLocal(member) && (f.Path == filepath.Join(content, member) || f.Path == filepath.Join(root, member)) {
				return f.Path
			}
		} else if f.Size > largest.Size {
			largest = f
		}
	}
	return largest.Path
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
	if ep.SourceModTime != 0 {
		return ep.SourceModTime == info.ModTime().UnixNano()
	}
	_, canVerify := dl.(downloader.FileLister)
	return dl != nil && !canVerify
}
