//go:build windows

package update

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"time"

	"golang.org/x/sys/windows"
)

func restoreExecutable(old, target string) error {
	// 已退出的程序可能仍被扫描器短暂占用；保留旧映像，直到替换成功或等待超时。
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := os.Rename(old, target)
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return err
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func watchUpdateParent(pid int) func() bool {
	// Windows 保留已退出父进程的 PID；绑定一次内核进程句柄，避免 PID 回收后误认新进程。
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return nil
	}
	var parent, child, exit, kernel, user windows.Filetime
	if windows.GetProcessTimes(h, &parent, &exit, &kernel, &user) != nil ||
		windows.GetProcessTimes(windows.CurrentProcess(), &child, &exit, &kernel, &user) != nil ||
		parent.Nanoseconds() > child.Nanoseconds() {
		windows.CloseHandle(h)
		return nil
	}
	// 每个受管服务仅持有一份句柄，服务退出时由操作系统关闭。
	return func() bool {
		event, err := windows.WaitForSingleObject(h, 0)
		return err == nil && event == uint32(windows.WAIT_TIMEOUT)
	}
}

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
