package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

func TestServesEveryBuiltFrontendAsset(t *testing.T) {
	handler := staticHandler()
	assets, err := os.ReadDir("web/dist/assets")
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range assets {
		if asset.IsDir() {
			continue
		}
		name := asset.Name()
		t.Run(name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("web", "dist", "assets", name))
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/assets/"+name, nil))
			if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), want) {
				t.Errorf("asset returned HTTP %d and %d bytes of %q, expected the %d-byte build output", response.Code, response.Body.Len(), response.Header().Get("Content-Type"), len(want))
			}
		})
	}
}

func TestFrontendNavigationAndAssetErrors(t *testing.T) {
	handler := staticHandler()
	for _, tc := range []struct {
		path   string
		status int
		html   bool
	}{
		{"/", 200, true}, {"/subscriptions/12", 200, true},
		{"/assets/missing-build-chunk.js", 404, false}, {"/missing-icon.png", 404, false},
		{"/manifest.json", 200, false}, {"/sw.js", 200, false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRecorder()
			handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if r.Code != tc.status || strings.HasPrefix(r.Header().Get("Content-Type"), "text/html") != tc.html {
				t.Fatalf("%s: HTTP %d, %s", tc.path, r.Code, r.Header().Get("Content-Type"))
			}
			if strings.Contains(r.Header().Get("Cache-Control"), "immutable") {
				t.Fatalf("固定地址或错误响应不可永久缓存: %s", tc.path)
			}
		})
	}
}

func TestManifestDeclaresExistingIconsAtTheirActualSize(t *testing.T) {
	manifest, err := os.ReadFile("web/public/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Icons []struct{ Src, Sizes, Type string }
	}
	if err := json.Unmarshal(manifest, &doc); err != nil {
		t.Fatal(err)
	}
	for _, icon := range doc.Icons {
		if icon.Type != "image/png" {
			continue
		}
		f, err := os.Open(filepath.Join("web/public", strings.TrimPrefix(icon.Src, "/")))
		if err != nil {
			t.Fatal(err)
		}
		size, err := png.DecodeConfig(f)
		f.Close()
		if err != nil || fmt.Sprintf("%dx%d", size.Width, size.Height) != icon.Sizes {
			t.Fatalf("图标 %s 声明 %s，实际尺寸 %dx%d: %v", icon.Src, icon.Sizes, size.Width, size.Height, err)
		}
	}
}

func TestViteBundleGzip(t *testing.T) {
	// 使用构建产物证明浏览器收到的字节和源资源完全相同。
	for _, ext := range []string{"css", "js"} {
		assets, err := filepath.Glob("web/dist/assets/index-*." + ext)
		if err != nil || len(assets) != 1 {
			t.Fatalf("entry %s: %v %v", ext, assets, err)
		}
		orig, err := os.ReadFile(assets[0])
		if err != nil {
			t.Fatal(err)
		}
		path := "/assets/" + filepath.Base(assets[0])
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		w := httptest.NewRecorder()
		staticHandler().ServeHTTP(w, req)
		if w.Code != 200 || w.Header().Get("Content-Encoding") != "gzip" || w.Header().Get("Vary") != "Accept-Encoding" {
			t.Fatalf("%s: %d %v", path, w.Code, w.Header())
		}
		z, err := gzip.NewReader(w.Body)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(z)
		if err != nil || !bytes.Equal(b, orig) {
			t.Fatalf("%s mismatch: %v", path, err)
		}
		if err = z.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestViteEncodingNegotiationAndRange(t *testing.T) {
	entries, err := filepath.Glob("web/dist/assets/index-*.js")
	if err != nil || len(entries) != 1 {
		t.Fatalf("entry: %v %v", entries, err)
	}
	entry := filepath.Base(entries[0])
	name := "/assets/" + entry
	original, err := os.ReadFile(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(filepath.Join("web", "dist", "assets", "gzip", entry+".gz"))
	if err != nil {
		t.Fatal(err)
	}
	handler := staticHandler()
	tests := []struct {
		label, method string
		encodings     []string
		rangeHeader   string
		code          int
		body          []byte
		encoding      string
	}{
		{"without header", "GET", nil, "", 200, original, ""},
		{"explicit gzip disabled", "GET", []string{"gzip;q=0, br"}, "", 200, original, ""},
		{"conflicting duplicate", "GET", []string{"gzip;q=0", "gzip"}, "", 200, original, ""},
		{"wildcard", "GET", []string{"br, *"}, "", 200, encoded, "gzip"},
		{"specific exclusion beats wildcard", "GET", []string{"*", "GZIP;Q=0"}, "", 200, original, ""},
		{"explicit identity preferred", "GET", []string{"identity;q=0.8, gzip;q=0.25"}, "", 200, original, ""},
		{"gzip preferred", "GET", []string{"identity;q=0.25, GZIP;q=0.8"}, "", 200, encoded, "gzip"},
		{"empty coding rejected", "GET", []string{"br, *;q=0"}, "", 406, nil, ""},
		{"invalid gzip weight", "GET", []string{"gzip;q=NaN"}, "", 200, original, ""},
		{"head", "HEAD", []string{"gzip"}, "", 200, encoded, "gzip"},
		{"range of identity", "GET", []string{"gzip"}, "bytes=2-5", 206, original[2:6], ""},
		{"gzip only ignores range", "GET", []string{"gzip, identity;q=0"}, "bytes=2-5", 200, encoded, "gzip"},
		{"invalid identity range", "GET", []string{"gzip"}, "bytes=999999999999-", 416, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, name, nil)
			for _, value := range tc.encodings {
				req.Header.Add("Accept-Encoding", value)
			}
			if tc.rangeHeader != "" {
				req.Header.Set("Range", tc.rangeHeader)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.code || w.Header().Get("Content-Encoding") != tc.encoding || w.Header().Get("Vary") != "Accept-Encoding" {
				t.Fatalf("status=%d headers=%v", w.Code, w.Header())
			}
			if tc.body == nil {
				if strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
					t.Fatal("error cached as immutable")
				}
				return
			}
			if w.Header().Get("Content-Length") != strconv.Itoa(len(tc.body)) || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
				t.Fatalf("wrong length or cache policy: %v", w.Header())
			}
			if tc.method == "HEAD" {
				if w.Body.Len() != 0 {
					t.Fatal("HEAD returned body")
				}
			} else if !bytes.Equal(w.Body.Bytes(), tc.body) {
				t.Fatal("incorrect representation")
			}
			if tc.code == 206 && w.Header().Get("Content-Range") != fmt.Sprintf("bytes 2-5/%d", len(original)) {
				t.Fatalf("range %s", w.Header().Get("Content-Range"))
			}
		})
	}
	for _, url := range []string{"/assets/gzip", "/assets/gzip/" + entry + ".gz", "/assets/gzip/unknown.gz", "/assets/missing-abcdefgh.js"} {
		req := httptest.NewRequest("GET", url, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 404 || w.Header().Get("Content-Encoding") != "" || strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
			t.Fatalf("reserved or missing path %s returned %d %v", url, w.Code, w.Header())
		}
	}
}

func TestViteEncodingWithoutSidecar(t *testing.T) {
	const name = "assets/plain-abcdefgh.js"
	files := fstest.MapFS{name: {Data: []byte("x")}}
	for _, tc := range []struct {
		accept   string
		rejected bool
	}{{"gzip", false}, {"identity;q=0, gzip", true}} {
		req := httptest.NewRequest("GET", "/"+name, nil)
		req.Header.Set("Accept-Encoding", tc.accept)
		w := httptest.NewRecorder()
		if handled := serveEncodedAsset(w, req, files, name); handled != tc.rejected || (handled && w.Code != 406) || w.Header().Get("Content-Encoding") != "" {
			t.Fatalf("%s: handled=%v status=%d", tc.accept, handled, w.Code)
		}
	}
}

func TestViteGzipPreconditions(t *testing.T) {
	entries, err := filepath.Glob("web/dist/assets/index-*.css")
	if err != nil || len(entries) != 1 {
		t.Fatalf("entry: %v %v", entries, err)
	}
	srv := httptest.NewServer(staticHandler())
	t.Cleanup(srv.Close)
	url := srv.URL + "/assets/" + filepath.Base(entries[0])
	for _, method := range []string{"GET", "HEAD"} {
		for _, tc := range []struct {
			header, value string
			wantStatus    int
		}{{"If-Match", `"previous-version"`, 412}, {"If-None-Match", "*", 304}} {
			t.Run(method+"/"+tc.header, func(t *testing.T) {
				req, err := http.NewRequest(method, url, nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Accept-Encoding", "gzip")
				req.Header.Set(tc.header, tc.value)
				resp, err := srv.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				length := resp.Header.Get("Content-Length")
				if err != nil || len(body) != 0 || resp.StatusCode != tc.wantStatus ||
					resp.Header.Get("Content-Encoding") != "" || (length != "" && length != "0") || resp.Header.Get("Vary") != "Accept-Encoding" {
					t.Fatalf("HTTP %d, headers %v, %d body bytes, read %v", resp.StatusCode, resp.Header, len(body), err)
				}
			})
		}
	}
}
