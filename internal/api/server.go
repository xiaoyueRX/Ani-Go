// Package api 提供 HTTP REST API 服务
// 包含 JWT 鉴权、路由注册、服务启动
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

// Server 持有 API 所需的依赖
type Server struct {
	downloader        core.Downloader
	triggerSupplement func(ctx context.Context, subID uint) error
	pluginManager     *plugin.Manager
	taskParser        core.TaskParser
	mikanSrc          *source.MikanSource  // Mikan 资源源，用于字幕组查询
	yucSrc            *source.YucWikiSource // yuc.wiki 资源源，用于时间表
}

// StartServer 启动 HTTP API 服务（支持优雅关闭）
// staticHandler 为嵌入式前端静态文件服务，若为 nil 则仅提供 API 服务
func StartServer(ctx context.Context, host string, port int, dl core.Downloader, triggerSupp func(ctx context.Context, subID uint) error, pluginMgr *plugin.Manager, parser core.TaskParser, staticHandler http.Handler) *http.Server {
	s := &Server{
		downloader:        dl,
		triggerSupplement: triggerSupp,
		pluginManager:     pluginMgr,
		taskParser:        parser,
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	apiHandler := auth.ProxyHeadersMiddleware(
		auth.CORSMiddleware(
			auth.AuthMiddleware(mux),
		),
	)

	// 将 API 处理器与静态文件处理器合并
	var finalHandler http.Handler
	if staticHandler != nil {
		finalHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// API 路由优先
			if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" {
				apiHandler.ServeHTTP(w, r)
				return
			}
			// /api/health 和 /api/login 也走 API
			if r.URL.Path == "/api/health" || r.URL.Path == "/api/login" || r.URL.Path == "/api/me" {
				apiHandler.ServeHTTP(w, r)
				return
			}
			// 其余全部交给静态文件处理器（含 SPA 回退）
			staticHandler.ServeHTTP(w, r)
		})
		log.Println("✅ 前端静态文件处理器已挂载（非 /api/* 路径 → SPA 回退）")
	} else {
		finalHandler = apiHandler
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      finalHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("🌐 HTTP 服务启动: http://%s", addr)
		log.Printf("   API 文档: http://%s/api/login (POST)", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ HTTP 服务异常退出: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		log.Println("🛑 HTTP 服务正在关闭...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	// 启动时自动测速，选择最快的 Mikan 镜像
	go func() {
		if s.mikanSrc == nil {
			return
		}
		time.Sleep(2 * time.Second)
		log.Println("📡 正在测速 Mikan 镜像...")
		results := s.mikanSrc.TestLatency(context.Background())
		best := source.BestDomain(results, s.mikanSrc.GetDomain())
		for _, r := range results {
			status := "✅"
			if !r.OK {
				status = "❌"
			}
			log.Printf("  %s %s: %dms", status, r.Domain, r.Latency)
		}
		if best != s.mikanSrc.GetDomain() {
			log.Printf("🚀 自动切换镜像: %s → %dms", best, func() int64 {
				for _, r := range results {
					if r.Domain == best {
						return r.Latency
					}
				}
				return 0
			}())
			s.mikanSrc.SetDomain(best)
		} else {
			log.Printf("✅ 当前镜像 %s 表现最佳", best)
		}
	}()

	return srv
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// 认证接口（AuthMiddleware 放行）
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/health", handleHealth)

	// 用户信息
	mux.HandleFunc("GET /api/me", handleMe)

	// 订阅管理 CRUD
	mux.HandleFunc("GET /api/subscriptions", s.handleListSubscriptions)
	mux.HandleFunc("POST /api/subscriptions", s.handleCreateSubscription)
	mux.HandleFunc("GET /api/subscriptions/{id}", s.handleGetSubscription)
	mux.HandleFunc("PUT /api/subscriptions/{id}", s.handleUpdateSubscription)
	mux.HandleFunc("DELETE /api/subscriptions/{id}", s.handleDeleteSubscription)
	mux.HandleFunc("POST /api/subscriptions/{id}/trigger-supplement", s.handleTriggerSupplement)

	// 剧集管理
	mux.HandleFunc("PUT /api/episodes/{id}/status", s.handleUpdateEpisodeStatus)

	// 下载队列
	mux.HandleFunc("GET /api/downloads", s.handleListDownloads)

	// 设置
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("GET /api/settings/custom-regex", s.handleGetCustomRegex)
	mux.HandleFunc("POST /api/settings/custom-regex/reload", s.handleReloadCustomRegex)

	// 插件管理
	mux.HandleFunc("GET /api/plugins", s.handleGetPlugins)
	mux.HandleFunc("POST /api/plugins/reload", s.handleReloadPlugins)

	// 数据迁移
	mux.HandleFunc("POST /api/migrate", s.handleMigrateData)

	// 任务解析
	mux.HandleFunc("POST /api/parse", s.handleParseTask)

	// 搜索番剧
	mux.HandleFunc("GET /api/search", s.handleSearchAnime)

	// Mikan 字幕组查询（根据 BangumiID 获取字幕组 RSS URL）
	s.mikanSrc = source.NewMikanSource("mikanime.tv", "", nil)
	mux.HandleFunc("GET /api/mikan/groups", s.handleMikanGroups)

	// 新番时间表（使用 yuc.wiki 数据源）
	s.yucSrc = source.NewYucWikiSource()
	mux.HandleFunc("GET /api/schedule", s.handleSchedule)

	// Mikan 镜像测速
	mux.HandleFunc("POST /api/mikan/test-mirrors", s.handleTestMirrors)
	mux.HandleFunc("POST /api/mikan/select-mirror", s.handleSelectMirror)
}

// ============================================================
// 请求/响应结构
// ============================================================

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Message  string `json:"message"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// ============================================================
// 认证处理器
// ============================================================

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "仅支持 POST"})
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "用户名和密码不能为空"})
		return
	}

	var user database.User
	if err := database.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		log.Printf("⚠️  登录失败: 用户 %s 不存在", req.Username)
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "用户名或密码错误"})
		return
	}

	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		log.Printf("⚠️  登录失败: 用户 %s 密码错误", req.Username)
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "用户名或密码错误"})
		return
	}

	token, err := auth.GenerateToken(req.Username)
	if err != nil {
		log.Printf("❌ JWT 签发失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Token 生成失败"})
		return
	}

	log.Printf("✅ 用户 %s 登录成功", req.Username)
	writeJSON(w, http.StatusOK, loginResponse{
		Token:    token,
		Username: req.Username,
		Message:  "登录成功",
	})
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	claims, err := auth.ValidateToken(token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Token 无效"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"username": claims.Username,
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// ============================================================
// 辅助函数
// ============================================================

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}
