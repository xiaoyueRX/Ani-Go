//go:build !windows

package downloader

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func getFreeDiskSpace(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	if stat.Bsize <= 0 {
		return 0, fmt.Errorf("无效的文件系统块大小: %d", stat.Bsize)
	}
	if stat.Bavail == 0 {
		return 0, nil
	}
	// Bavail 是非特权用户可分配的空闲块数
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
