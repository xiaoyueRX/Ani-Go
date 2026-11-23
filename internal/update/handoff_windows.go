//go:build windows

package update

import (
	"os"
	"os/exec"
	"runtime"
	"time"
)

// Windows 旧进程退出后由管道通知交接程序，此时服务端口与程序句柄均已释放。
func launchHandoff(old string, args []string) error {
	read, write, err := os.Pipe()
	if err != nil {
		return err
	}
	defer read.Close()
	cmd := exec.Command(old, args...)
	cmd.Stdin = read
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		write.Close()
		return err
	}
	_ = cmd.Process.Release()
	go func() { time.Sleep(time.Second); runtime.KeepAlive(write); os.Exit(0) }()
	return nil
}
