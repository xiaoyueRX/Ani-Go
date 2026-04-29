package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

//go:embed web/dist
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

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 尝试打开请求的文件
		path := r.URL.Path

		// 根路径直接返回 index.html
		if path == "/" {
			path = "/index.html"
		}

		f, err := staticFS.Open(path[1:]) // 去掉前导 /
		if err != nil {
			// 文件不存在，回退到 index.html（SPA 路由）
			indexData, indexErr := fs.ReadFile(staticFS, "index.html")
			if indexErr != nil {
				http.Error(w, "内部错误", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(indexData)
			return
		}
		f.Close()

		// 文件存在，正常服务
		fileServer.ServeHTTP(w, r)
	})
}
