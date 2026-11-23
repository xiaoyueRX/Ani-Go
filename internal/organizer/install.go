package organizer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// MoveSource 是已发布库文件所对应的待清理下载源。可随整理记录持久化。
type MoveSource struct {
	Path    string
	ModTime int64
}

// Cleanup 在媒体库文件仍然完整且下载路径未被替换时完成移动。
func (m MoveSource) Cleanup(ctx context.Context, dst string) error {
	src, err := os.Lstat(m.Path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	target, err := os.Lstat(dst)
	if err != nil {
		return err
	}
	if !src.Mode().IsRegular() || !target.Mode().IsRegular() || src.ModTime().UnixNano() != m.ModTime || src.Size() != target.Size() {
		return fmt.Errorf("媒体或源文件已变化，保留源文件: %s", m.Path)
	}
	if sameDirectoryEntry(m.Path, dst) {
		return nil
	}
	if !os.SameFile(src, target) {
		equal, err := SameMediaContents(ctx, m.Path, dst)
		if err != nil {
			return err
		}
		if !equal {
			return fmt.Errorf("源和媒体库内容不同，保留源文件: %s", m.Path)
		}
	}
	current, err := os.Lstat(m.Path)
	if err != nil {
		return err
	}
	if !os.SameFile(src, current) || src.ModTime() != current.ModTime() || src.Size() != current.Size() {
		return fmt.Errorf("清理前源文件已变更: %s", m.Path)
	}
	published, err := os.Lstat(dst)
	if err != nil {
		return err
	}
	if !os.SameFile(target, published) || target.Size() != published.Size() || target.ModTime() != published.ModTime() {
		return fmt.Errorf("清理前媒体库文件已变更: %s", dst)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(m.Path); err != nil {
		return err
	}
	return syncMediaDir(filepath.Dir(m.Path))
}

func sameDirectoryEntry(src, dst string) bool {
	sourceName, targetName := filepath.Base(src), filepath.Base(dst)
	if !strings.EqualFold(sourceName, targetName) {
		return false
	}
	a, err := os.Stat(filepath.Dir(src))
	if err != nil {
		return false
	}
	b, err := os.Stat(filepath.Dir(dst))
	if err != nil || !os.SameFile(a, b) {
		return false
	}
	if sourceName == targetName {
		return true
	}
	// 区分大小写的卷允许同目录存在两个仅大小写不同的独立目录项。
	entries, err := os.ReadDir(filepath.Dir(src))
	if err != nil {
		return true
	}
	var sourcePresent, targetPresent bool
	for _, entry := range entries {
		sourcePresent = sourcePresent || entry.Name() == sourceName
		targetPresent = targetPresent || entry.Name() == targetName
	}
	return !(sourcePresent && targetPresent)
}

// SameMediaContents 逐段核对两个常规文件的大小和全部字节，供跨卷恢复使用。
func SameMediaContents(ctx context.Context, src, dst string) (bool, error) {
	a, err := os.Open(src)
	if err != nil {
		return false, err
	}
	defer a.Close()
	b, err := os.Open(dst)
	if err != nil {
		return false, err
	}
	defer b.Close()
	ai, err := a.Stat()
	if err != nil {
		return false, err
	}
	bi, err := b.Stat()
	if err != nil {
		return false, err
	}
	if !ai.Mode().IsRegular() || !bi.Mode().IsRegular() || ai.Size() != bi.Size() {
		return false, nil
	}
	if os.SameFile(ai, bi) {
		return true, nil
	}
	x, y := make([]byte, 128<<10), make([]byte, 128<<10)
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		n, e := a.Read(x)
		if n > 0 {
			if _, err := io.ReadFull(b, y[:n]); err != nil {
				return false, err
			}
			if !bytes.Equal(x[:n], y[:n]) {
				return false, nil
			}
		}
		if errors.Is(e, io.EOF) {
			return true, nil
		}
		if e != nil {
			return false, e
		}
	}
}

// 新建目录后同步父目录，保证提交库记录前路径已经落盘。
func makeMediaDir(dir string) error {
	_, err := os.Stat(dir)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(dir)
	if err := makeMediaDir(parent); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0755); err != nil && !os.IsExist(err) {
		return err
	}
	return syncMediaDir(parent)
}

func syncMediaDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// installMedia 在目标卷准备文件，发布前始终保留源及已有目标。
func installMedia(ctx context.Context, src, dst string, publish func(string, string) error) error {
	stage, release, err := newMediaStage(filepath.Dir(dst), lockStageDirectory)
	if err != nil {
		return err
	}
	defer release()
	tmp := filepath.Join(stage, "media")
	if err := os.Link(src, tmp); err != nil {
		if err := copyFile(ctx, src, tmp); err != nil {
			return err
		}
	} else if runtime.GOOS != "windows" {
		f, err := os.Open(tmp)
		if err != nil {
			return err
		}
		err = f.Sync()
		_ = f.Close()
		if err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return publish(tmp, dst)
}

type mediaReader struct {
	ctx context.Context
	r   io.Reader
}

func (r mediaReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func copyFile(ctx context.Context, srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	before, err := src.Stat()
	if err != nil {
		return err
	}
	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(dst, mediaReader{ctx, src})
	after, err := src.Stat()
	if copyErr == nil && err == nil && (n != before.Size() || n != after.Size() || before.ModTime() != after.ModTime()) {
		copyErr = fmt.Errorf("复制期间源文件已变更: %s", srcPath)
	}
	if copyErr == nil {
		copyErr = err
	}
	if copyErr == nil {
		copyErr = dst.Sync()
	}
	return errors.Join(copyErr, dst.Close())
}
