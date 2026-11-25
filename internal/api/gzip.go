package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

var gzipWriterPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		return w
	},
}

// shouldBypassGzip 判断请求是否属于 SSE 或二进制媒体流，这些路径不适用缓冲压缩。
func shouldBypassGzip(r *http.Request) bool {
	if r.Method == http.MethodHead || !strings.Contains(strings.ToLower(r.Header.Get("Accept-Encoding")), "gzip") {
		return true
	}
	p := r.URL.Path
	switch {
	case p == "/api/events/stream",
		p == "/mcp/sse",
		p == "/api/proxy/image",
		p == "/api/user/avatar",
		strings.HasPrefix(p, "/api/backup/download/"):
		return true
	}
	return false
}

func isCompressibleContentType(ct string) bool {
	if ct == "" {
		return true
	}
	lower := strings.ToLower(ct)
	return strings.Contains(lower, "application/json") ||
		strings.HasPrefix(lower, "text/") && !strings.Contains(lower, "text/event-stream") ||
		strings.Contains(lower, "application/javascript") ||
		strings.Contains(lower, "application/xml")
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gw          *gzip.Writer
	decided     bool
	compress    bool
	wroteHeader bool
}

func (g *gzipResponseWriter) decide(statusCode int) {
	if g.decided {
		return
	}
	g.decided = true

	h := g.ResponseWriter.Header()
	if statusCode == http.StatusNoContent || statusCode == http.StatusNotModified ||
		h.Get("Content-Encoding") != "" || !isCompressibleContentType(h.Get("Content-Type")) {
		g.compress = false
		return
	}

	g.compress = true
	h.Set("Content-Encoding", "gzip")
	h.Add("Vary", "Accept-Encoding")
	h.Del("Content-Length")

	gw := gzipWriterPool.Get().(*gzip.Writer)
	gw.Reset(g.ResponseWriter)
	g.gw = gw
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.wroteHeader {
		return
	}
	g.wroteHeader = true
	g.decide(code)
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.wroteHeader {
		if g.ResponseWriter.Header().Get("Content-Type") == "" && len(b) > 0 {
			g.ResponseWriter.Header().Set("Content-Type", http.DetectContentType(b))
		}
		g.WriteHeader(http.StatusOK)
	}
	if g.compress && g.gw != nil {
		return g.gw.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func (g *gzipResponseWriter) Flush() {
	if g.compress && g.gw != nil {
		_ = g.gw.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (g *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return g.ResponseWriter
}

func (g *gzipResponseWriter) close() {
	if g.gw != nil {
		_ = g.gw.Close()
		g.gw.Reset(io.Discard)
		gzipWriterPool.Put(g.gw)
		g.gw = nil
	}
}

// gzipMiddleware 为 JSON 与文本类 API 响应启用池化 Gzip 压缩，显著降低局域网与远程 Web 访问带宽。
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldBypassGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}
