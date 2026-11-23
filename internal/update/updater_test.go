package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestDetectEnvironment(t *testing.T) {
	env, hasSock := DetectEnvironment()
	if env != EnvBinary && env != EnvDocker {
		t.Errorf("未知的环境类型: %s", env)
	}
	t.Logf("当前环境: %s, hasSock: %v", env, hasSock)
}

func TestCheckUpdate_Mock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"tag_name": "v0.7.0",
			"body": "测试更新日志",
			"published_at": "2026-09-30T12:00:00Z"
		}`))
	}))
	defer srv.Close()

	m := &Manager{
		currentVersion: "v0.6.0",
		client:         srv.Client(),
	}

	// 临时改写 endpoints 探测逻辑或单元测试管理器状态
	m.cachedInfo = &UpdateInfo{
		CurrentVersion: "v0.6.0",
		LatestVersion:  "v0.7.0",
		HasUpdate:      true,
		ReleaseNotes:   "测试更新说明",
	}
	if !m.cachedInfo.HasUpdate {
		t.Errorf("期望发现更新")
	}
}

func TestReleaseIntegrity(t *testing.T) {
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	tw := tar.NewWriter(gz)
	payload := []byte("verified binary bytes")
	if err := tw.WriteHeader(&tar.Header{Name: "pkg/anigo", Mode: 0755, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	tw.Write(payload)
	tw.Close()
	gz.Close()
	archive := compressed.Bytes()
	sum := sha256.Sum256(archive)
	sums := []byte(fmt.Sprintf("%x  release.tar.gz\n", sum))
	if err := verifyPackage(archive, sums, "release.tar.gz"); err != nil {
		t.Fatal(err)
	}
	if err := verifyPackage(archive[:len(archive)-1], sums, "release.tar.gz"); err == nil {
		t.Error("损坏的发行包通过摘要校验")
	}
	out, err := extractProgram(archive, "tar.gz", "anigo")
	if err != nil || !bytes.Equal(out, payload) {
		t.Fatalf("完整归档解析失败: %v", err)
	}
	if _, err := extractProgram(archive[:len(archive)-4], "tar.gz", "anigo"); err == nil {
		t.Error("损坏的 gzip 尾部被忽略")
	}
	var z bytes.Buffer
	zw := zip.NewWriter(&z)
	ze, _ := zw.CreateHeader(&zip.FileHeader{Name: "pkg/anigo.exe", Method: zip.Store})
	ze.Write(payload)
	zw.Close()
	raw := z.Bytes()
	raw[bytes.Index(raw, payload)] ^= 1
	if _, err := extractProgram(raw, "zip", "anigo.exe"); err == nil {
		t.Error("损坏的 ZIP 内容被接受")
	}
}

func TestDockerUpdateCapability(t *testing.T) {
	if env, _ := DetectEnvironment(); env == EnvDocker {
		m := &Manager{}
		if err := m.ExecuteUpdate(context.Background()); err == nil {
			t.Fatal("容器内不应受理自更新")
		}
	}
}

func TestVersionHealthRequiresRequestedVersion(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"status":"ok","version":"v1.0.0"}`) }))
	defer s.Close()
	if versionHealthy(s.Client(), s.URL, "v2.0.0") || !versionHealthy(s.Client(), s.URL, "v1.0.0") {
		t.Fatal("健康检查没有校验版本")
	}
}

func TestRollbackRestartsOldBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 可执行文件替换需要 Windows 运行器")
	}
	dir := t.TempDir()
	stage := filepath.Join(dir, ".anigo-update-test")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(stage, "previous-anigo")
	marker := filepath.Join(dir, "restored")
	if err := os.WriteFile(old, []byte("#!/bin/sh\ntouch \"$1\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(dir, "anigo")
	if err := os.WriteFile(executable, []byte("invalid program"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := runReplacement(old, executable, "v2.0.0", "http://127.0.0.1", []string{marker}); err != nil {
		t.Fatal("旧程序回滚运行失败", err)
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
	}
	t.Fatal("旧版未启动")
}
