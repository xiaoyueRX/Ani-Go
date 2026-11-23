package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const handoffFlag = "--anigo-update-handoff"
const monitorControl = "ANIGO_INTERNAL_UPDATE_CONTROL"
const monitorOwner = "ANIGO_INTERNAL_UPDATE_MONITOR_PID"
const nextUpdateExit = 75

// 服务启动时绑定父进程；旧监管版本只传控制路径，仍然沿用原退出码和 JSON 协议。
var monitorAlive = bindUpdateMonitor()

func bindUpdateMonitor() func() bool {
	if os.Getenv(monitorControl) == "" || len(os.Args) > 1 && os.Args[1] == handoffFlag {
		return nil
	}
	pid := os.Getppid()
	if owner := os.Getenv(monitorOwner); pid <= 0 || owner != "" && owner != strconv.Itoa(pid) {
		return nil
	}
	return watchUpdateParent(pid)
}

type replacement struct {
	Old     string
	Version string
}

// 复用监管进程时先允许请求返回；延迟交接的最终错误由本次更新的调用者接收。
func startHandoff(executable string, program []byte, version, healthURL string) (<-chan error, error) {
	stage, err := os.MkdirTemp(filepath.Dir(executable), ".anigo-update-")
	if err != nil {
		return nil, err
	}
	installed := false
	defer func() {
		if !installed {
			_ = os.RemoveAll(stage)
		}
	}()
	candidate := filepath.Join(stage, filepath.Base(executable))
	old := filepath.Join(stage, "previous-"+filepath.Base(executable))
	if err := os.WriteFile(candidate, program, 0755); err != nil {
		return nil, err
	}
	// 旧进程运行期间先保留原程序，随后的失败都可以回退。
	if runtime.GOOS == "windows" {
		if err := os.Rename(executable, old); err != nil {
			return nil, err
		}
	} else if err := os.Link(executable, old); err != nil {
		if err := copyExecutable(executable, old); err != nil {
			return nil, err
		}
	}
	rollback := func(cause error) error {
		if err := restoreExecutable(old, executable); err != nil {
			installed = true
			return fmt.Errorf("%v；旧版保存在 %s，恢复失败: %w", cause, old, err)
		}
		return cause
	}
	if err := os.Rename(candidate, executable); err != nil {
		if runtime.GOOS == "windows" {
			return nil, rollback(err)
		}
		return nil, err
	}
	installed = true
	freshHandoff := func() error {
		args := append([]string{handoffFlag, executable, version, healthURL}, os.Args[1:]...)
		if err := launchHandoff(old, args); err != nil {
			return rollback(err)
		}
		return nil
	}
	// 已有监管进程继续复用同一 PID 和控制文件，防止连续升级累积后台进程。
	if control := os.Getenv(monitorControl); monitorAlive != nil && monitorAlive() && filepath.Dir(filepath.Dir(control)) == filepath.Dir(executable) && strings.HasPrefix(filepath.Base(filepath.Dir(control)), ".anigo-monitor-") {
		data, err := json.Marshal(replacement{Old: old, Version: version})
		if err == nil {
			err = os.WriteFile(control, data, 0600)
		}
		if err != nil {
			if !monitorAlive() {
				return nil, freshHandoff()
			}
			return nil, rollback(err)
		}
		result := make(chan error, 1)
		go func() {
			defer close(result)
			time.Sleep(time.Second)
			if monitorAlive() {
				os.Exit(nextUpdateExit)
			}
			// 写入交接记录后监管进程仍可能退出，此时由当前服务启动新的监管进程。
			result <- freshHandoff()
		}()
		return result, nil
	}
	return nil, freshHandoff()
}

func copyExecutable(src, dst string) error {
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// RunHandoff 仅处理更新子进程。新版健康后才确认更新，失败时恢复旧程序。
func RunHandoff(args []string) bool {
	if len(args) < 1 || args[0] != handoffFlag {
		return false
	}
	if len(args) < 4 {
		log.Print("更新交接参数无效")
		return true
	}
	old, err := os.Executable()
	if err != nil {
		log.Printf("读取旧版路径失败: %v", err)
		return true
	}
	if filepath.Base(old) != "previous-"+filepath.Base(args[1]) || filepath.Dir(filepath.Dir(old)) != filepath.Dir(args[1]) {
		log.Print("更新交接的程序路径无效")
		return true
	}
	if runtime.GOOS == "windows" {
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			log.Printf("更新交接失败: %v", err)
			return true
		}
	}
	if err := runReplacement(old, args[1], args[2], args[3], args[4:]); err != nil {
		log.Printf("更新交接失败: %v", err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
	return true
}

func runReplacement(old, executable, version, healthURL string, args []string) error {
	// Windows 首次交接使用旧映像运行监管进程，之后各次升级的旧映像可以回收。
	supervisorImage := old
	monitor, err := os.MkdirTemp(filepath.Dir(executable), ".anigo-monitor-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(monitor)
	control := filepath.Join(monitor, "next.json")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	for {
		cmd := exec.Command(executable, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), monitorControl+"="+control, monitorOwner+"="+strconv.Itoa(os.Getpid()))
		startErr := cmd.Start()
		if startErr == nil {
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			// 回滚后的旧版沿用原监管进程，只等待它退出。
			confirmed := old == ""
			var exitErr error
			confirmed, exitErr = monitorProgram(cmd, done, signals, client, healthURL, version, confirmed, func() {
				if old != "" && (runtime.GOOS != "windows" || old != supervisorImage) {
					_ = os.Remove(old)
					_ = os.Remove(filepath.Dir(old))
				}
			})
			if confirmed {
				var status *exec.ExitError
				if !errors.As(exitErr, &status) || status.ExitCode() != nextUpdateExit {
					return exitErr
				}
				data, err := os.ReadFile(control)
				if err != nil {
					return exitErr
				}
				var next replacement
				if err := json.Unmarshal(data, &next); err != nil {
					return err
				}
				if filepath.Dir(filepath.Dir(next.Old)) != filepath.Dir(executable) || filepath.Base(next.Old) != "previous-"+filepath.Base(executable) {
					return fmt.Errorf("下次升级的回滚路径无效")
				}
				if err := os.Remove(control); err != nil {
					return err
				}
				old = next.Old
				version = next.Version
				continue
			}
			startErr = exitErr
		}
		if old == "" {
			return startErr
		}
		if err := restoreExecutable(old, executable); err != nil {
			return fmt.Errorf("%v；旧版位于 %s，回滚失败: %w", startErr, old, err)
		}
		log.Printf("新版未通过健康检查，已恢复旧版: %v", startErr)
		_ = os.Remove(filepath.Dir(old))
		old = ""
	}
}

func monitorProgram(cmd *exec.Cmd, done <-chan error, signals <-chan os.Signal, client *http.Client, healthURL, version string, confirmed bool, onHealthy func()) (bool, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	var ticks <-chan time.Time = ticker.C
	var expires <-chan time.Time = deadline.C
	if confirmed {
		ticks = nil
		expires = nil
	}
	for {
		select {
		case err := <-done:
			if !confirmed && err == nil {
				err = fmt.Errorf("新版在健康检查前退出")
			}
			return confirmed, err
		case sig := <-signals:
			_ = cmd.Process.Signal(sig)
			return true, <-done
		case <-expires:
			_ = cmd.Process.Kill()
			<-done
			return false, fmt.Errorf("新版健康检查超时")
		case <-ticks:
			if versionHealthy(client, healthURL, version) {
				log.Printf("新版 %s 已通过健康检查", version)
				onHealthy()
				confirmed = true
				ticks = nil
				expires = nil
			}
		}
	}
}

func versionHealthy(client *http.Client, u, version string) bool {
	res, err := client.Get(u)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var health struct {
		Status  string
		Version string
	}
	return res.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&health) == nil && health.Status == "ok" && health.Version == version
}
