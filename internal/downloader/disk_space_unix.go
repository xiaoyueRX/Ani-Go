//go:build !windows

package downloader

import (
	"golang.org/x/sys/unix"
)

func getFreeDiskSpace(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	// Bavail 是非特权用户可分配的空闲块数
	return stat.Bavail * uint64(stat.Bsize), nil
}
