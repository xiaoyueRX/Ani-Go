package main

import (
	"bytes"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"path"
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
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(staticFS, name); err == nil && info.Mode().IsRegular() {
				if asset {
					// Vite 构建资源的名称包含内容哈希。
					w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
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
