package organizer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const stagePrefix = ".anigo-stage-v1-"
const stageOwner = "Ani-Go media staging v1\n"

// 每次整理独占随机目录。支持目录锁的卷可在下一次整理时回收上次进程留下的大文件。
func newMediaStage(dir string, tryLock func(*os.File) bool) (string, func(), error) {
	if matches, err := filepath.Glob(filepath.Join(dir, stagePrefix+"*")); err == nil {
		for _, stage := range matches {
			reclaimMediaStage(stage, tryLock)
		}
	}
	stage, err := os.MkdirTemp(dir, stagePrefix)
	if err != nil {
		return "", nil, err
	}
	lock, err := os.Open(stage)
	if err == nil && tryLock(lock) {
		if err := os.WriteFile(filepath.Join(stage, "owner"), []byte(stageOwner), 0600); err != nil {
			lock.Close()
			_ = os.Remove(filepath.Join(stage, "owner"))
			_ = os.Remove(stage)
			return "", nil, err
		}
	} else if lock != nil {
		_ = lock.Close()
		lock = nil
	}
	release := func() {
		_ = os.Remove(filepath.Join(stage, "media"))
		_ = os.Remove(filepath.Join(stage, "owner"))
		_ = os.Remove(stage)
		if lock != nil {
			_ = lock.Close()
		}
	}
	return stage, release, nil
}

func reclaimMediaStage(path string, tryLock func(*os.File) bool) {
	suffix, ok := strings.CutPrefix(filepath.Base(path), stagePrefix)
	if !ok || suffix == "" || strings.Trim(suffix, "0123456789") != "" {
		return
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return
	}
	// 锁标记在加锁后写入。新目录未完成初始化时最多只留下空目录。
	marker, err := os.Lstat(filepath.Join(path, "owner"))
	if err != nil || !marker.Mode().IsRegular() || marker.Size() != int64(len(stageOwner)) {
		return
	}
	lock, err := os.Open(path)
	if err != nil {
		return
	}
	defer lock.Close()
	if !tryLock(lock) {
		return
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return
	}
	defer root.Close()
	locked, err := lock.Stat()
	if err != nil {
		return
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(locked, opened) || !os.SameFile(info, opened) {
		return
	}
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || (entry.Name() != "owner" && entry.Name() != "media") {
			return
		}
	}
	data, err := root.ReadFile("owner")
	if err != nil || string(data) != stageOwner {
		return
	}
	if err := root.Remove("media"); err != nil && !os.IsNotExist(err) {
		return
	}
	if err := root.Remove("owner"); err != nil {
		return
	}
	if current, err := os.Lstat(path); err == nil && os.SameFile(current, locked) {
		_ = os.Remove(path)
	}
}
