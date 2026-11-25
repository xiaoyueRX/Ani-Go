package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGzipMiddleware_CompressesJSONAndSkipsStreams(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Repeat(`{"title":"Ani-Go"}`, 10))
	})
	mux.HandleFunc("/api/events/stream", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Fatal("SSE 路由必须保留 http.Flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"connected\"}\n\n")
	})
	mux.HandleFunc("/api/proxy/image", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("RAW_JPEG_DATA"))
	})

	handler := gzipMiddleware(mux)

	// 1. JSON + Accept-Encoding: gzip 应返回可解压的 gzip 响应
	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("JSON 响应未启用 gzip: Content-Encoding=%q", w.Header().Get("Content-Encoding"))
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("缺少 Vary: Accept-Encoding 头")
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("创建 gzip 解压器失败: %v", err)
	}
	defer zr.Close()
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("解压响应失败: %v", err)
	}
	if string(decoded) != strings.Repeat(`{"title":"Ani-Go"}`, 10) {
		t.Fatalf("解压内容不一致: %q", string(decoded))
	}

	// 2. SSE 不得启用 gzip
	sseReq := httptest.NewRequest(http.MethodGet, "/api/events/stream", nil)
	sseReq.Header.Set("Accept-Encoding", "gzip")
	sseW := httptest.NewRecorder()
	handler.ServeHTTP(sseW, sseReq)
	if sseW.Header().Get("Content-Encoding") != "" {
		t.Fatalf("SSE 路由不应启用 gzip: %q", sseW.Header().Get("Content-Encoding"))
	}
	if sseW.Body.String() != "data: {\"type\":\"connected\"}\n\n" {
		t.Fatalf("SSE 内容异常: %q", sseW.Body.String())
	}

	// 3. 图片代理不得启用 gzip
	imgReq := httptest.NewRequest(http.MethodGet, "/api/proxy/image", nil)
	imgReq.Header.Set("Accept-Encoding", "gzip")
	imgW := httptest.NewRecorder()
	handler.ServeHTTP(imgW, imgReq)
	if imgW.Header().Get("Content-Encoding") != "" || imgW.Body.String() != "RAW_JPEG_DATA" {
		t.Fatalf("图片代理不应被 gzip 压缩: enc=%q body=%q", imgW.Header().Get("Content-Encoding"), imgW.Body.String())
	}
}
