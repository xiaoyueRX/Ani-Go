//go:build !windows

package update

import (
	"os"
	"syscall"
)

// 用同一进程号接管服务，保证 systemd 或 supervisord 继续监管正确的进程。
func launchHandoff(old string, args []string) error {
	return syscall.Exec(old, append([]string{old}, args...), os.Environ())
}
