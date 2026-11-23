package organizer

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

const repairReadFlags = unix.O_NONBLOCK

func linkRepairSource(source *os.File, _ string, _ *os.Root, parent *os.File, name string, validate func() error) error {
	if err := validate(); err != nil {
		return err
	}
	// 空路径直接引用已打开文件；普通账号可经 procfs 对同一描述符建立链接。
	err := unix.Linkat(int(source.Fd()), "", int(parent.Fd()), name, unix.AT_EMPTY_PATH)
	if errors.Is(err, unix.EPERM) || errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EINVAL) {
		err = unix.Linkat(unix.AT_FDCWD, "/proc/self/fd/"+strconv.FormatUint(uint64(source.Fd()), 10), int(parent.Fd()), name, unix.AT_SYMLINK_FOLLOW)
	}
	if errors.Is(err, unix.EXDEV) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EMLINK) {
		return fmt.Errorf("%w: %w", ErrLinkUnavailable, err)
	}
	return err
}
