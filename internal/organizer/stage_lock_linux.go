//go:build linux

package organizer

import (
	"os"

	"golang.org/x/sys/unix"
)

// 远程共享盘可能只在本机模拟锁；只有能判定为本地挂载的卷参与跨进程回收。
func localStageLockSupported(kind uint32) bool {
	switch kind {
	case unix.EXT4_SUPER_MAGIC, unix.BTRFS_SUPER_MAGIC, unix.XFS_SUPER_MAGIC, unix.F2FS_SUPER_MAGIC, unix.TMPFS_MAGIC:
		return true
	default:
		return false
	}
}

func lockStageDirectory(dir *os.File) bool {
	fd := int(dir.Fd())
	var stat unix.Statfs_t
	return unix.Fstatfs(fd, &stat) == nil && localStageLockSupported(uint32(stat.Type)) && unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) == nil
}
