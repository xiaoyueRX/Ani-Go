package main

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//go:embed all:web/dist
var distFS embed.FS

// staticFS 去除 "web/dist" 前缀后的静态文件系统
var staticFS fs.FS

func init() {
	var err error
	staticFS, err = fs.Sub(distFS, "web/dist")
	if err != nil {
		log.Fatalf("❌ 嵌入文件系统初始化失败: %v", err)
	}
	log.Println("✅ 前端静态文件已嵌入二进制 (go:embed web/dist)")
}

var viteTextAsset = regexp.MustCompile(`^assets/[^/]+-[A-Za-z0-9_-]{8}\.(css|js)$`)

// assetEncodingQuality 比较显式编码的权重，星号只补足没有点名的编码。
func assetEncodingQuality(values []string) (gzip, identity float64) {
	gzip, identity, wildcard := -1.0, -1.0, -1.0
	for _, line := range values {
		for item := range strings.SplitSeq(line, ",") {
			coding, param, hasParam := strings.Cut(item, ";")
			var weight *float64
			switch strings.ToLower(strings.TrimSpace(coding)) {
			case "gzip":
				weight = &gzip
			case "identity":
				weight = &identity
			case "*":
				weight = &wildcard
			default:
				continue
			}
			q := 1.0
			if hasParam {
				key, value, ok := strings.Cut(param, "=")
				n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
				if !ok || !strings.EqualFold(strings.TrimSpace(key), "q") || err != nil || !(n >= 0 && n <= 1) {
					q = 0
				} else {
					q = n
				}
			}
			if *weight < 0 || q < *weight {
				*weight = q
			}
		}
	}
	if gzip < 0 {
		gzip = max(wildcard, 0)
	}
	if identity < 0 {
		identity = 1
		if wildcard == 0 {
			identity = 0
		}
	}
	return gzip, identity
}

// 文件服务器先处理条件请求并计算表示长度；只在实际传输压缩文件时标注编码。
type gzipAssetResponse struct {
	http.ResponseWriter
	contentType string
}

func (w gzipAssetResponse) WriteHeader(status int) {
	if status == http.StatusOK {
		w.Header().Set("Content-Type", w.contentType)
		w.Header().Set("Content-Encoding", "gzip")
	} else if status >= 400 {
		w.Header().Del("Cache-Control")
	}
	w.ResponseWriter.WriteHeader(status)
}

// 保留底层 ResponseWriter 的文件传输优化。
func (w gzipAssetResponse) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(w.ResponseWriter, r)
}

// 仅为请求的公开哈希文件协商表示。Range 默认针对原始字节；只接受 gzip 时发送完整压缩表示。
func serveEncodedAsset(w http.ResponseWriter, r *http.Request, files fs.FS, name string) bool {
	w.Header().Add("Vary", "Accept-Encoding")
	gzip, identity := assetEncodingQuality(r.Header.Values("Accept-Encoding"))
	hasRange := r.Header.Get("Range") != ""
	if gzip > 0 && gzip >= identity && (!hasRange || identity == 0) {
		sidecar := "assets/gzip/" + path.Base(name) + ".gz"
		if info, err := fs.Stat(files, sidecar); err == nil && info.Mode().IsRegular() {
			if hasRange {
				r = r.Clone(r.Context())
				r.Header.Del("Range")
			}
			http.ServeFileFS(gzipAssetResponse{w, mime.TypeByExtension(path.Ext(name))}, r, files, sidecar)
			return true
		}
	}
	if identity == 0 {
		w.Header().Del("Cache-Control")
		http.Error(w, "没有可接受的内容编码", http.StatusNotAcceptable)
		return true
	}
	return false
}

// staticHandler 处理 SPA 静态文件请求
// 优先匹配精确文件，未找到时回退到 index.html（Vue Router History 模式）
func staticHandler() http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))
	index, indexErr := fs.ReadFile(staticFS, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "仅支持 GET 或 HEAD", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		asset := strings.HasPrefix(name, "assets/")
		if strings.HasPrefix(name, "assets/gzip/") {
			http.NotFound(w, r)
			return
		}
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(staticFS, name); err == nil && info.Mode().IsRegular() {
				if asset {
					// Vite 构建资源的名称包含内容哈希。
					w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
					if viteTextAsset.MatchString(name) && serveEncodedAsset(w, r, staticFS, name) {
						return
					}
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
			if asset || (!strings.Contains(name, "/") && path.Ext(name) != "") ||
				name == "api" || strings.HasPrefix(name, "api/") || name == "mcp" || strings.HasPrefix(name, "mcp/") {
				http.NotFound(w, r)
				return
			}
		}
		if indexErr != nil {
			http.Error(w, "前端入口不可用", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	})
}
