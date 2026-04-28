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
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// StartServer 启动 HTTP API 服务（支持优雅关闭）
func StartServer(ctx context.Context, host string, port int) *http.Server {
	mux := http.NewServeMux()

	// 注册路由
	registerRoutes(mux)

	// 中间件链: ProxyHeaders → CORS → Auth
	handler := auth.ProxyHeadersMiddleware(
		auth.CORSMiddleware(
			auth.AuthMiddleware(mux),
		),
	)

	addr := fmt.Sprintf("%s:%d", host, port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
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

	// 优雅关闭
	go func() {
		<-ctx.Done()
		log.Println("🛑 HTTP 服务正在关闭...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	return srv
}

func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/me", handleMe)
	mux.HandleFunc("/api/health", handleHealth)
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
// API 处理器
// ============================================================

// handleLogin 处理登录请求，校验密码并签发 JWT
// POST /api/login
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

	// 查询用户
	var user database.User
	if err := database.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		log.Printf("⚠️  登录失败: 用户 %s 不存在", req.Username)
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "用户名或密码错误"})
		return
	}

	// 校验 Bcrypt 密码
	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		log.Printf("⚠️  登录失败: 用户 %s 密码错误", req.Username)
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "用户名或密码错误"})
		return
	}

	// 签发 JWT
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

// handleMe 获取当前登录用户信息（需认证）
// GET /api/me
func handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "仅支持 GET"})
		return
	}

	// 从 Authorization 头提取用户信息
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

// handleHealth 健康检查接口
// GET /api/health
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
