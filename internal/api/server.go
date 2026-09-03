// Package api 提供 HTTP REST API 服务
// 包含 JWT 鉴权、路由注册、服务启动
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/backup"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
	"github.com/xiaoyueRX/Ani-Go/internal/search"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
	"github.com/xiaoyueRX/Ani-Go/internal/notifier/v2"
)

// Server 持有 API 所需的依赖
type Server struct {
	downloader        core.Downloader
	triggerSupplement func(ctx context.Context, subID uint) error
	pluginManager     *plugin.Manager
	taskParser        core.TaskParser
	mikanSrc          *source.MikanSource // Mikan 资源源，用于字幕组查询
	multiSrc          core.Source         // 聚合资源源，用于搜索
	smartExpander     *search.Expander
	smartAggregator   *search.Aggregator
	yucSrc            *source.YucWikiSource // yuc.wiki 资源源，用于时间表
	version           string
	logPath           string
	eventBus          core.EventBus
	notifyMgr         *v2.NotifyManager
	backupManager     *backup.BackupManager // 备份管理器
	md                core.MetadataProvider
}

// StartServer 启动 HTTP API 服务（支持优雅关闭）
// staticHandler 为嵌入式前端静态文件服务，若为 nil 则仅提供 API 服务
type ServerOptions struct {
	SmartSearchEnabled bool
	AIChat             ai.Classifier
	BackupManager      *backup.BackupManager
}

func StartServer(ctx context.Context, host string, port int, version string, allowedOrigins []string, dl core.Downloader, triggerSupp func(ctx context.Context, subID uint) error, pluginMgr *plugin.Manager, parser core.TaskParser, mikan *source.MikanSource, yuc *source.YucWikiSource, multi core.Source, staticHandler http.Handler, logPath string, notifyMgr *v2.NotifyManager, md core.MetadataProvider, bus core.EventBus, options ...ServerOptions) *http.Server {
	opts := ServerOptions{}
	if len(options) > 0 {
		opts = options[0]
	}

	bm := opts.BackupManager
	if bm == nil {
		bm = backup.NewBackupManager(&config.Config{})
	}

	s := &Server{
		downloader:        dl,
		triggerSupplement: triggerSupp,
		pluginManager:     pluginMgr,
		taskParser:        parser,
		mikanSrc:          mikan,
		yucSrc:            yuc,
		multiSrc:          multi,
		eventBus:          bus,
		version:           version,
		smartExpander:     search.NewExpander(opts.AIChat, opts.SmartSearchEnabled),
		smartAggregator:   search.NewAggregator(multi),
		logPath:           logPath,
		notifyMgr:         notifyMgr,
		backupManager:     bm,
		md:                md,
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	apiHandler := auth.ProxyHeadersMiddleware(
		auth.CORSMiddleware(allowedOrigins)(
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
			if r.URL.Path == "/api/health" || r.URL.Path == "/api/login" || r.URL.Path == "/api/me" || strings.HasPrefix(r.URL.Path, "/api/proxy/image") {
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

func (s *Server) SetBackupManager(bm *backup.BackupManager) {
	s.backupManager = bm
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// 认证接口（AuthMiddleware 放行）
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/health", handleHealth)

	// 用户信息与头像
	mux.HandleFunc("GET /api/me", handleMe)
	mux.HandleFunc("POST /api/user/change-password", s.handleChangePassword)
	mux.HandleFunc("POST /api/user/avatar", s.handleUploadAvatar)
	mux.HandleFunc("GET /api/user/avatar", s.handleGetAvatar)

	// 订阅管理 CRUD
	mux.HandleFunc("GET /api/subscriptions", s.handleListSubscriptions)
	mux.HandleFunc("POST /api/subscriptions", s.handleCreateSubscription)
	mux.HandleFunc("POST /api/subscriptions/batch", s.handleBatchCreateSubscriptions)
	mux.HandleFunc("GET /api/subgroups", s.handleGetSubgroups)
	mux.HandleFunc("GET /api/subscriptions/{id}", s.handleGetSubscription)
	mux.HandleFunc("PUT /api/subscriptions/{id}", s.handleUpdateSubscription)
	mux.HandleFunc("DELETE /api/subscriptions/{id}", s.handleDeleteSubscription)
	mux.HandleFunc("POST /api/subscriptions/batch-delete", s.handleBatchDeleteSubscriptions)
	mux.HandleFunc("POST /api/subscriptions/batch-restore", s.handleBatchRestoreSubscriptions)
	mux.HandleFunc("POST /api/subscriptions/{id}/trigger-supplement", s.handleTriggerSupplement)
	mux.HandleFunc("POST /api/subscriptions/supplement-all", s.handleSupplementAll)

	// 剧集管理
	mux.HandleFunc("PUT /api/episodes/{id}/status", s.handleUpdateEpisodeStatus)

	// 下载队列
	mux.HandleFunc("GET /api/downloads", s.handleListDownloads)

	// 设置
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("GET /api/settings/custom-regex", s.handleGetCustomRegex)
	mux.HandleFunc("POST /api/settings/custom-regex/reload", s.handleReloadCustomRegex)
	mux.HandleFunc("GET /api/logs", s.handleGetLogs)

	// 备份管理
	mux.HandleFunc("GET /api/backup/list", s.handleListBackups)
	mux.HandleFunc("POST /api/backup/create", s.handleCreateBackup)
	mux.HandleFunc("POST /api/backup/restore", s.handleRestoreBackup)
	mux.HandleFunc("DELETE /api/backup/{name}", s.handleDeleteBackup)
	mux.HandleFunc("GET /api/backup/download/{name}", s.handleDownloadBackup)

	// 通知测试
	mux.HandleFunc("POST /api/notify/test", s.handleTestNotify)

	// 数据迁移
	mux.HandleFunc("POST /api/migrate", s.handleMigrateData)

	// 任务解析
	mux.HandleFunc("POST /api/parse", s.handleParseTask)

	// 搜索番剧
	mux.HandleFunc("GET /api/search", s.handleSearchAnime)
	mux.HandleFunc("POST /api/search/smart", s.handleSmartSearch)

	// Mikan 字幕组查询（根据 BangumiID 获取字幕组 RSS URL）
	mux.HandleFunc("GET /api/mikan/groups", s.handleMikanGroups)

	// 插件管理
	mux.HandleFunc("GET /api/plugins", s.handleListPlugins)
	mux.HandleFunc("POST /api/plugins/save", s.handleSavePlugin)
	mux.HandleFunc("DELETE /api/plugins/{id}", s.handleDeletePlugin)
	mux.HandleFunc("POST /api/plugins/toggle", s.handleTogglePlugin)
	mux.HandleFunc("POST /api/plugins/reload", s.handleReloadPlugins)

	// 新番时间表（使用 yuc.wiki 数据源）
	mux.HandleFunc("GET /api/schedule", s.handleSchedule)
	mux.HandleFunc("GET /api/schedule/bangumi", s.handleScheduleBangumi)
	mux.HandleFunc("GET /api/bangumi/subject/", s.handleGetBangumiSubject)
	mux.HandleFunc("GET /api/bangumi/auth/link", s.handleBangumiAuthLink)
	mux.HandleFunc("GET /api/bangumi/auth/callback", s.handleBangumiAuthCallback)

	// Mikan 镜像测速
	mux.HandleFunc("POST /api/mikan/test-mirrors", s.handleTestMirrors)
	mux.HandleFunc("POST /api/bgm/test-mirrors", s.handleBGMTestMirrors)
	mux.HandleFunc("POST /api/mikan/select-mirror", s.handleSelectMirror)
	mux.HandleFunc("POST /api/bgm/select-mirror", s.handleBGMSelectMirror)

	// AI 模型列表
	mux.HandleFunc("POST /api/ai/models", s.handleGetAIModels)

	// 图片代理（Bilibili CDN 热链保护绕过）
	mux.HandleFunc("GET /api/proxy/image", s.handleProxyImage)

	// 版本信息
	mux.HandleFunc("GET /api/version", s.handleGetVersion)

	// 事件流 (SSE)
	mux.HandleFunc("GET /api/events/stream", s.handleEventStream)
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

	token, err := auth.GenerateToken(req.Username, user.TokenVersion)
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

	var user database.User
	if err := database.DB.Where("username = ?", claims.Username).First(&user).Error; err != nil {
		writeJSON(w, http.StatusOK, map[string]string{
			"username":  claims.Username,
			"avatar_url": "",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"username":  claims.Username,
		"avatar_url": user.AvatarURL,
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

// ============================================================
// 备份管理 API
// ============================================================

// handleListBackups 列出所有备份文件
// GET /api/backup/list
func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	if s.backupManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "备份管理器未初始化"})
		return
	}

	files, err := s.backupManager.ListBackups()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, files)
}

// handleCreateBackup 手动创建备份
// POST /api/backup/create body: {"include_episodes": false}
func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	if s.backupManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "备份管理器未初始化"})
		return
	}

	var req struct {
		IncludeEpisodes bool `json:"include_episodes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	if err := s.backupManager.CreateBackup(ctx, req.IncludeEpisodes); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "备份创建成功"})
}

// handleRestoreBackup 从备份恢复
// POST /api/backup/restore body: {"name": "anigo_backup_20260830_full.json"}
func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	if s.backupManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "备份管理器未初始化"})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "备份文件名不能为空"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	if err := s.backupManager.RestoreBackup(ctx, req.Name); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "恢复成功，建议重启服务生效"})
}

// handleDeleteBackup 删除备份文件
// DELETE /api/backup/{name}
func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	if s.backupManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "备份管理器未初始化"})
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "备份文件名不能为空"})
		return
	}

	if err := s.backupManager.DeleteBackup(name); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "备份文件已删除"})
}

// handleDownloadBackup 下载备份文件
// GET /api/backup/download/{name}
func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	if s.backupManager == nil {
		http.Error(w, "备份管理器未初始化", http.StatusServiceUnavailable)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "备份文件名不能为空", http.StatusBadRequest)
		return
	}

	filePath := s.backupManager.GetBackupPath(name)
	if filePath == "" {
		http.Error(w, "备份文件不存在", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))
	w.Header().Set("Content-Type", "application/json")
	http.ServeFile(w, r, filePath)
}
