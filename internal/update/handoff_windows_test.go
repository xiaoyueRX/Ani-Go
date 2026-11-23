//go:build windows

package update

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const windowsTestService = "--anigo-test-service"
const newImageMarker = "\nAni-Go test release v1.0.1\n"
const nextImageMarker = "\nAni-Go test release v1.0.2\n"

type windowsFixtureState struct {
	Status, Version, Arch, Note string
	PID, Parent                 int
	Probed                      bool
}

func TestMain(m *testing.M) {
	if RunHandoff(os.Args[1:]) {
		return
	}
	if len(os.Args) == 2 && os.Args[1] == windowsTestService {
		if err := serveWindowsUpdateFixture(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	os.Exit(m.Run())
}

// 运行真实 PE 和生产交接逻辑。末尾标记区分两份可执行映像，不依赖更换外部配置。
func serveWindowsUpdateFixture() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	image, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	version := "v1.0.0"
	if bytes.HasSuffix(image, []byte(newImageMarker)) {
		version = "v1.0.1"
	} else if bytes.HasSuffix(image, []byte(nextImageMarker)) {
		version = "v1.0.2"
	}
	note, err := os.ReadFile("data.txt")
	if err != nil {
		return err
	}
	addr := os.Getenv("ANIGO_WINDOWS_TEST_ADDR")
	var probed atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		probed.Store(true)
		fmt.Fprintf(w, `{"status":"ok","version":"%s"}`, version)
	})
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(windowsFixtureState{"ok", version, runtime.GOARCH, string(note), os.Getpid(), os.Getppid(), probed.Load()})
	})
	mux.HandleFunc("POST /update", func(w http.ResponseWriter, _ *http.Request) {
		candidate, err := os.ReadFile("candidate.bin")
		if err == nil {
			requested := "v1.0.1"
			if version == "v1.0.1" {
				requested = "v1.0.2"
			}
			err = startHandoff(exe, candidate, requested, "http://"+addr+"/api/health")
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	return http.ListenAndServe(addr, mux)
}

func TestWindowsBinaryUpdate(t *testing.T) {
	testExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	oldImage, err := os.ReadFile(testExe)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, version string
		healthy       bool
	}{
		{"healthy", "v1.0.1", true},
		{"rollback", "v1.0.0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			exe := filepath.Join(root, "动漫 服务.exe")
			candidate := []byte("invalid executable")
			if tc.healthy {
				candidate = append(bytes.Clone(oldImage), newImageMarker...)
			}
			const note = "保留订阅 S02"
			for name, data := range map[string][]byte{exe: oldImage, filepath.Join(root, "candidate.bin"): candidate, filepath.Join(root, "data.txt"): []byte(note)} {
				if err := os.WriteFile(name, data, 0700); err != nil {
					t.Fatal(err)
				}
			}
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			address := listener.Addr().String()
			listener.Close()
			cmd := exec.Command(exe, windowsTestService)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "ANIGO_WINDOWS_TEST_ADDR="+address)
			// 文件句柄避免监管子进程继承日志管道导致 Wait 一直阻塞。
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			runWindowsFixture(t, cmd)
			client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}}
			defer client.CloseIdleConnections()
			base := "http://" + address
			await := func(wantVersion string, replaced bool) windowsFixtureState {
				t.Helper()
				var state windowsFixtureState
				for until := time.Now().Add(30 * time.Second); time.Now().Before(until); time.Sleep(100 * time.Millisecond) {
					res, err := client.Get(base + "/state")
					if err != nil {
						continue
					}
					err = json.NewDecoder(res.Body).Decode(&state)
					res.Body.Close()
					if err == nil && state.Version == wantVersion && state.PID > 0 && (state.PID != cmd.Process.Pid) == replaced &&
						(!replaced || !tc.healthy || state.Probed) {
						return state
					}
				}
				t.Fatalf("未等到预期进程 %s，最后状态 %+v", wantVersion, state)
				return state
			}
			apply := func() {
				t.Helper()
				res, err := client.Post(base+"/update", "text/plain", nil)
				if err != nil {
					t.Fatal(err)
				}
				message, err := io.ReadAll(res.Body)
				res.Body.Close()
				if err != nil || res.StatusCode != 200 {
					t.Fatalf("交接请求状态 %d: %s, %v", res.StatusCode, message, err)
				}
			}
			before := await("v1.0.0", false)
			apply()
			after := await(tc.version, true)
			if after.Note != note || after.Arch != runtime.GOARCH || after.Parent == before.Parent || after.Parent == before.PID {
				t.Fatalf("交接后的进程或持久记录不匹配: %+v → %+v", before, after)
			}
			installed, err := os.ReadFile(exe)
			expected := oldImage
			if tc.healthy {
				expected = candidate
			}
			if err != nil || !bytes.Equal(installed, expected) {
				t.Fatalf("原安装路径中的程序与目标版本不符: %v", err)
			}
			t.Logf("windows/%s: PID %d → %d, supervisor %d, version %s", after.Arch, before.PID, after.PID, after.Parent, after.Version)
			if !tc.healthy {
				return
			}
			next := append(bytes.Clone(oldImage), nextImageMarker...)
			if err := os.WriteFile(filepath.Join(root, "candidate.bin"), next, 0700); err != nil {
				t.Fatal(err)
			}
			apply()
			last := await("v1.0.2", true)
			if last.PID == after.PID || last.Parent != after.Parent || last.Note != note {
				t.Fatalf("连续升级后的监管进程或持久记录不匹配: %+v → %+v", after, last)
			}
			// 首次交接的监管程序本身来自第一个备份目录，后续旧版可在新版本健康后删除。
			var copies []string
			for until := time.Now().Add(5 * time.Second); time.Now().Before(until); time.Sleep(50 * time.Millisecond) {
				copies, err = filepath.Glob(filepath.Join(root, ".anigo-update-*", "previous-"+filepath.Base(exe)))
				if err != nil {
					t.Fatal(err)
				}
				if len(copies) <= 1 {
					return
				}
			}
			t.Fatalf("连续两次确认更新后残留了 %d 份旧程序: %v", len(copies), copies)
		})
	}
}

// 下载目录被短暂扫描时，旧版仍应在原安装路径恢复并正常退出。
func TestWindowsRollbackAfterTemporarySharingViolation(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, ".anigo-update-test")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(stage, "previous-anigo.exe")
	running, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := copyExecutable(running, old); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "anigo.exe")
	if err := os.WriteFile(target, []byte("invalid program"), 0600); err != nil {
		t.Fatal(err)
	}
	path, err := windows.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	unlocked := make(chan error, 1)
	time.AfterFunc(750*time.Millisecond, func() { unlocked <- windows.CloseHandle(handle) })
	err = runReplacement(old, target, "v0.0.2", "http://127.0.0.1:1", []string{"-test.run=^$"})
	if closeErr := <-unlocked; closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatalf("临时占用解除后未能恢复旧程序: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("回滚文件未被移回安装路径: %v", err)
	}
}

// 把父服务和它创建的监管/替换进程收在独立 Job 中，失败断言也能清理。
func runWindowsFixture(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { windows.CloseHandle(job) })
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := windows.TerminateJobObject(job, 0); err != nil {
			t.Errorf("清理更新测试进程: %v", err)
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	p, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(p)
	if err := windows.AssignProcessToJobObject(job, p); err != nil {
		t.Fatal(err)
	}
}
