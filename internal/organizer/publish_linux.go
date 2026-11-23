package organizer

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// 在同一目标卷内原子发布。旧内核或文件系统不支持排他重命名时尝试硬链接。
func publishNewFile(src, dst string) error {
	err := unix.Renameat2(unix.AT_FDCWD, src, unix.AT_FDCWD, dst, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		return os.Link(src, dst)
	}
	return err
}
