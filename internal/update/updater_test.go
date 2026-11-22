package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCheckOfficialRelease(t *testing.T) {
	m := Manager{currentVersion: "v0.6.0", client: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.github.com" {
			t.Fatalf("版本来源错误: %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tag_name":"v0.7.0","body":"test"}`))}, nil
	})}}
	got, err := m.CheckUpdate(context.Background())
	if err != nil || !got.HasUpdate || got.LatestVersion != "v0.7.0" || got.CanSelfUpdate != (got.Environment == "binary") {
		t.Fatalf("检查结果: %+v, %v", got, err)
	}
}

func TestUpdateRejectsUnverifiedArchive(t *testing.T) {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	arch := runtime.GOARCH
	if arch == "arm" {
		arch = "armv7"
	}
	name := fmt.Sprintf("ani-go-v1.0.0-%s-%s.%s", runtime.GOOS, arch, ext)
	for _, digest := range []string{"", "sha256:" + strings.Repeat("0", 64)} {
		fetched := false
		m := Manager{currentVersion: "v0.7.0", client: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
			body := "损坏的归档"
			if req.URL.Host == "api.github.com" {
				body = fmt.Sprintf(`{"tag_name":"v1.0.0","assets":[{"name":%q,"digest":%q,"size":%d}]}`, name, digest, len("损坏的归档"))
			} else {
				fetched = true
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}}
		if err := m.updateBinary(context.Background()); err == nil || !strings.Contains(err.Error(), "SHA-256") || fetched != (digest != "") {
			t.Fatalf("校验错误: %v, 已下载=%v", err, fetched)
		}
	}
}

func TestInstallBinaryRetainsPriorVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "anigo")
	if err := os.WriteFile(path, []byte("原版"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := installBinary(path, []byte("新版")); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{path: "新版", path + ".old": "原版"} {
		got, err := os.ReadFile(file)
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q, %v", file, got, err)
		}
	}
}

func TestUnpackBinaryChecksArchive(t *testing.T) {
	payload := []byte("test binary contents")
	for _, ext := range []string{"tar.gz", "zip"} {
		t.Run(ext, func(t *testing.T) {
			var b bytes.Buffer
			if ext == "zip" {
				z := zip.NewWriter(&b)
				file, err := z.CreateHeader(&zip.FileHeader{Name: "release/anigo.exe", Method: zip.Store})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := file.Write(payload); err != nil {
					t.Fatal(err)
				}
				if err := z.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				g := gzip.NewWriter(&b)
				tw := tar.NewWriter(g)
				if err := tw.WriteHeader(&tar.Header{Name: "release/anigo", Mode: 0755, Size: int64(len(payload))}); err != nil {
					t.Fatal(err)
				}
				if _, err := tw.Write(payload); err != nil {
					t.Fatal(err)
				}
				if err := tw.Close(); err != nil {
					t.Fatal(err)
				}
				if err := g.Close(); err != nil {
					t.Fatal(err)
				}
			}
			data := b.Bytes()
			got, err := unpackBinary(data, ext)
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("解包: %q %v", got, err)
			}
			if ext == "zip" {
				data[bytes.Index(data, payload)] ^= 1
			} else {
				data[len(data)-1] ^= 1
			}
			if _, err := unpackBinary(data, ext); err == nil {
				t.Fatal("损坏的归档应被拒绝")
			}
		})
	}
}
