package organizer

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const repairReadFlags = unix.O_NONBLOCK

func linkRepairSource(source *os.File, sourcePath string, parent *os.Root, parentFD *os.File, name string, validate func() error) error {
	// macOS 的 linkat 需要源名。在已固定的目标卷独占暂存一次，先核对 inode，再排他发布。
	staging := ".anigo-repair-" + rand.Text()
	if err := parent.Mkdir(staging, 0700); err != nil {
		return err
	}
	stage, err := parent.OpenRoot(staging)
	if err != nil {
		return err
	}
	defer stage.Close()
	dir, err := stage.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	own, err := dir.Stat()
	if err != nil {
		return err
	}
	defer func() {
		_ = stage.Remove("media")
		if current, err := parent.Lstat(staging); err == nil && os.SameFile(own, current) {
			_ = parent.Remove(staging)
		}
	}()
	link := func(oldfd int, old string, newfd int, newname string) error {
		err := unix.Linkat(oldfd, old, newfd, newname, 0)
		if errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EMLINK) {
			return fmt.Errorf("%w: %w", ErrLinkUnavailable, err)
		}
		return err
	}
	if err := link(unix.AT_FDCWD, sourcePath, int(dir.Fd()), "media"); err != nil {
		return err
	}
	actual, err := stage.Lstat("media")
	if err != nil {
		return err
	}
	original, err := source.Stat()
	if err != nil {
		return err
	}
	if !actual.Mode().IsRegular() || !os.SameFile(actual, original) {
		return errors.New("暂存文件与下载原件不一致")
	}
	if err := validate(); err != nil {
		return err
	}
	return link(int(dir.Fd()), "media", int(parentFD.Fd()), name)
}
