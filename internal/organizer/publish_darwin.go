package organizer

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func publishNewFile(src, dst string) error {
	err := unix.RenamexNp(src, dst, unix.RENAME_EXCL)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) {
		return os.Link(src, dst)
	}
	return err
}
