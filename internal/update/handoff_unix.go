//go:build !windows

package update

import (
	"os"
	"syscall"
)

func restoreExecutable(old, target string) error {
	return os.Rename(old, target)
}

func watchUpdateParent(pid int) func() bool {
	// 父进程退出后，内核会把服务交给其他进程收养。
	return func() bool { return os.Getppid() == pid }
}

// 用同一进程号接管服务，保证 systemd 或 supervisord 继续监管正确的进程。
func launchHandoff(old string, args []string) error {
	return syscall.Exec(old, append([]string{old}, args...), os.Environ())
}
