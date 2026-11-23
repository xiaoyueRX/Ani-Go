package organizer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

// ErrLinkUnavailable 表示下载源与库之间无法建立硬链接。此操作不会退化为跨卷复制。
var ErrLinkUnavailable = errors.New("该文件系统或挂载不支持在原路径建立硬链接")

// RestoreVolume 从指定连接读取当前类别的生效库根。调用方可在同一事务中核对剧集所有权。
func (o *TVOrganizer) RestoreVolume(kind string, tx *gorm.DB) (LibraryVolume, error) {
	key, path, fallback := "TV_BASE_PATH", o.tvBasePath, "./TV/Media/番剧"
	switch normalizeAnimeType(kind) {
	case "movie":
		key, path, fallback = "MOVIE_BASE_PATH", o.movieBasePath, "./TV/Media/剧场版"
	case "ova":
		key, path, fallback = "OVA_BASE_PATH", o.ovaBasePath, "./TV/Media/OVA"
	}
	if path == "" {
		path = fallback
	}
	v := LibraryVolume{Root: path, ID: volumeID(kind)}
	if tx != nil {
		var setting database.Setting
		if err := tx.Where("key = ?", key).Limit(1).Find(&setting).Error; err != nil {
			return v, err
		}
		if setting.Value != "" {
			v.Root = setting.Value
		}
	}
	return v, nil
}

// RestoreLibraryLink 只补回已记录的库文件。目标父目录必须存在，已有目录项不会被替换。
func (v LibraryVolume) RestoreLibraryLink(ctx context.Context, ep database.Episode, source string, expectedRoot os.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	member := filepath.FromSlash(ep.SourceFile)
	if ep.Status != "organized" || !ep.HasBoundReviewFile() || ep.FileSize <= 0 || ep.SourceModTime == 0 ||
		!filepath.IsAbs(ep.DownloadRoot) || !filepath.IsAbs(source) || !core.IsVideoFile(member) ||
		member != filepath.Clean(member) || source != filepath.Join(ep.DownloadRoot, member) {
		return errors.New("缺少已绑定原件的本机路径、大小或修改时间")
	}
	base, err := filepath.Abs(v.Root)
	if err != nil {
		return err
	}
	target, valid := recordedMediaPath(v.Root, base, ep.FinalPath)
	if !valid || filepath.Dir(base) == base {
		return errors.New("历史目标不在当前媒体库中")
	}
	library, err := v.openRoot()
	if err != nil {
		return err
	}
	defer library.close()
	rootInfo, err := library.root.Stat(".")
	if err != nil {
		return err
	}
	if expectedRoot == nil || !os.SameFile(expectedRoot, rootInfo) {
		return errors.New("媒体库根目录自核对后已更换")
	}
	if err := library.checkRoot(ep.FinalPath); err != nil {
		return err
	}
	relDir, name := filepath.Dir(target), filepath.Base(target)
	parent, err := library.root.OpenRoot(relDir)
	if err != nil {
		return err
	}
	defer parent.Close()
	parentFD, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer parentFD.Close()
	parentInfo, err := parentFD.Stat()
	if err != nil {
		return err
	}
	download, err := (LibraryVolume{Root: ep.DownloadRoot}).openRoot()
	if err != nil {
		return err
	}
	defer download.close()
	if err := download.checkRoot(source); err != nil {
		return err
	}
	info, err := download.root.Lstat(ep.SourceFile)
	if err != nil {
		return err
	}
	if !matchesRepairSource(info, ep) {
		return errors.New("下载原件与保存的大小或修改时间不符")
	}
	original, err := download.root.OpenFile(ep.SourceFile, repairReadFlags, 0)
	if err != nil {
		return err
	}
	defer original.Close()
	pinned, err := original.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, pinned) {
		return errors.New("打开下载原件时文件已更换")
	}
	validate := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := library.checkRoot(ep.FinalPath); err != nil {
			return err
		}
		current, err := library.root.Stat(relDir)
		if err != nil {
			return err
		}
		if !os.SameFile(parentInfo, current) {
			return errors.New("媒体库目标父目录已更换")
		}
		if err := download.checkRoot(source); err != nil {
			return err
		}
		current, err = download.root.Lstat(ep.SourceFile)
		if err != nil {
			return err
		}
		if !matchesRepairSource(current, ep) || !os.SameFile(current, pinned) {
			return errors.New("下载原件在补回期间变化")
		}
		return nil
	}
	if err := validate(); err != nil {
		return err
	}
	_, err = parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		err = linkRepairSource(original, source, parent, parentFD, name, validate)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	} else if err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := parentFD.Sync(); err != nil {
			return err
		}
	}
	if err := validate(); err != nil {
		return err
	}
	entry, err := parent.Lstat(name)
	if err != nil {
		return err
	}
	if !entry.Mode().IsRegular() || !os.SameFile(entry, pinned) {
		return errors.New("历史目标已由另一个文件占用")
	}
	return nil
}

func matchesRepairSource(info os.FileInfo, ep database.Episode) bool {
	return info.Mode().IsRegular() && info.Size() == ep.FileSize && info.ModTime().UnixNano() == ep.SourceModTime
}
