package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
