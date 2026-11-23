package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const legacyMonitorFixture = "ANIGO_TEST_LEGACY_MONITOR_SERVICE"

// 旧监管程序只传控制文件路径，只理解退出码 75 和原来的两个 JSON 字段。
func TestLegacyMonitorProcess(t *testing.T) {
	exe := os.Getenv(legacyMonitorFixture)
	if exe == "" {
		return
	}
	// 等父测试完成进程组或 Windows Job 的登记，再创建后代。
	for {
		if _, err := os.Stat("start"); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	start := func() error {
		cmd := exec.Command(exe, "-test.run=^TestLegacyMonitoredService$")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		cmd.Env = append(os.Environ(), monitorControl+"="+filepath.Join(filepath.Dir(exe), ".anigo-monitor-legacy", "next.json"))
		return cmd.Run()
	}
	var exited *exec.ExitError
	if err := start(); !errors.As(err, &exited) || exited.ExitCode() != 75 {
		t.Fatalf("旧监管程序收到意外的退出状态: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(".anigo-monitor-legacy", "next.json"))
	var next struct{ Old, Version string }
	if err != nil || json.Unmarshal(data, &next) != nil || next.Version != "v2" || filepath.Base(next.Old) != "previous-"+filepath.Base(exe) {
		t.Fatalf("旧协议收到意外的交接记录 %q: %v", data, err)
	}
	if err := start(); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyMonitoredService(t *testing.T) {
	exe := os.Getenv(legacyMonitorFixture)
	if exe == "" {
		return
	}
	addr := os.Getenv("ANIGO_TEST_MONITOR_ADDR")
	if _, err := os.Stat("updating"); os.IsNotExist(err) {
		if err := os.WriteFile("ready", []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		for {
			if _, err := os.Stat("updating"); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		image, err := os.ReadFile(exe)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := startHandoff(exe, image, "v2", "http://"+addr+"/api/health"); err != nil {
			t.Fatal(err)
		}
		select {}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"ok","version":"v2"}`)
	})
	mux.HandleFunc("/owner", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(os.Getppid())
	})
	t.Fatal(http.ListenAndServe(addr, mux))
}

func TestUpdateWithLegacyMonitor(t *testing.T) {
	for _, state := range []string{"alive", "terminated", "terminated after request"} {
		t.Run(state, func(t *testing.T) {
			dir := t.TempDir()
			legacyDir := filepath.Join(dir, ".anigo-monitor-legacy")
			if err := os.Mkdir(legacyDir, 0700); err != nil {
				t.Fatal(err)
			}
			current, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			exe := filepath.Join(dir, "service"+filepath.Ext(current))
			if err := copyExecutable(current, exe); err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := listener.Addr().String()
			listener.Close()
			cmd := exec.Command(current, "-test.run=^TestLegacyMonitorProcess$")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), legacyMonitorFixture+"="+exe, "ANIGO_TEST_MONITOR_ADDR="+addr)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			startLegacyMonitorFixture(t, cmd)
			if err := os.WriteFile(filepath.Join(dir, "start"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			wait := func(label string, check func() bool) {
				t.Helper()
				for until := time.Now().Add(15 * time.Second); time.Now().Before(until); time.Sleep(25 * time.Millisecond) {
					if check() {
						return
					}
				}
				t.Fatal(label)
			}
			wait("受管服务未启动", func() bool { _, err := os.Stat(filepath.Join(dir, "ready")); return err == nil })
			terminate := func() {
				t.Helper()
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				if _, err := cmd.Process.Wait(); err != nil {
					t.Fatal(err)
				}
			}
			if state == "terminated" {
				terminate()
			}
			if err := os.WriteFile(filepath.Join(dir, "updating"), []byte("v2"), 0600); err != nil {
				t.Fatal(err)
			}
			if state == "terminated after request" {
				wait("交接记录未写入", func() bool {
					_, err := os.Stat(filepath.Join(legacyDir, "next.json"))
					return err == nil
				})
				terminate()
			}
			client := &http.Client{Timeout: 300 * time.Millisecond, Transport: &http.Transport{Proxy: nil}}
			defer client.CloseIdleConnections()
			base := "http://" + addr
			wait("升级后新服务未恢复", func() bool { return versionHealthy(client, base+"/api/health", "v2") })
			resp, err := client.Get(base + "/owner")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var pid int
			if err := json.NewDecoder(resp.Body).Decode(&pid); err != nil || pid <= 0 || (pid == cmd.Process.Pid) != (state == "alive") {
				t.Fatalf("新版父进程 %d、旧监管进程 %d (%s): %v", pid, cmd.Process.Pid, state, err)
			}
		})
	}
}
