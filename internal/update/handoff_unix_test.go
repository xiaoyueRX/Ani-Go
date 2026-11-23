//go:build !windows

package update

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if RunHandoff(os.Args[1:]) {
		return
	}
	os.Exit(m.Run())
}

func TestUpdateTestService(t *testing.T) {
	endpoint := os.Getenv("ANIGO_TEST_UPDATE_ENDPOINT")
	if endpoint == "" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	marker := executable + ".version"
	health := "http://" + endpoint + "/api/health"
	if os.Getenv(monitorControl) == "" {
		if err := os.WriteFile(marker, []byte("v1.0.1"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := startHandoff(executable, image, "v1.0.1", health); err != nil {
			t.Fatal(err)
		}
		return
	}
	version, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","version":"%s"}`, version)
	})
	mux.HandleFunc("/next", func(w http.ResponseWriter, _ *http.Request) {
		if err := os.WriteFile(marker, []byte("v1.0.2"), 0600); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err := startHandoff(executable, image, "v1.0.2", health); err != nil {
			http.Error(w, err.Error(), 500)
		}
	})
	srv := &http.Server{Addr: endpoint, Handler: mux}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()
	go func() { <-ctx.Done(); _ = srv.Close() }()
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		t.Fatal(err)
	}
}

func TestSupervisorKeepsPIDAcrossUpdates(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	l.Close()
	fixture := filepath.Join(t.TempDir(), "anigo")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := copyExecutable(binary, fixture); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(fixture, "-test.run=^TestUpdateTestService$")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(), "ANIGO_TEST_UPDATE_ENDPOINT="+address)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	const updateTimeout = 12 * time.Second
	healthClient := &http.Client{Timeout: 500 * time.Millisecond}
	defer healthClient.CloseIdleConnections()
	url := "http://" + address
	await := func(v string) {
		t.Helper()
		deadline := time.Now().Add(updateTimeout)
		for time.Now().Before(deadline) {
			select {
			case err := <-finished:
				t.Fatalf("监管主进程提前退出: %v", err)
			default:
			}
			if versionHealthy(healthClient, url+"/api/health", v) {
				// 监管进程确认新版本后才会移除回滚文件。
				backups, err := fs.Glob(os.DirFS(filepath.Dir(fixture)), ".anigo-update-*/previous-anigo")
				if err != nil {
					t.Fatal(err)
				}
				if len(backups) == 0 {
					return
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("监管进程未确认新版本 %s", v)
	}
	await("v1.0.1")
	// 升级请求要写入整个程序文件，应使用独立于健康探针的超时。
	updateClient := &http.Client{Timeout: updateTimeout}
	defer updateClient.CloseIdleConnections()
	res, err := updateClient.Post(url+"/next", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
	await("v1.0.2")
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("受监管服务未响应停止信号")
	}
}
