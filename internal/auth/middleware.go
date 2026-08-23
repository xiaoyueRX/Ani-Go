package auth

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// CORSMiddleware 处理跨域请求
// allowedOrigins 非空时进行 Origin 白名单校验，为空则允许所有来源
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	// 预构建 origin 集合用于快速查找
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				origin = r.Header.Get("Referer")
			}

			if len(originSet) > 0 {
					// 白名单模式：仅允许配置的 Origin
					if originSet[origin] {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
					}
				} else {
					// 无白名单时默认仅允许本地访问（安全优先）
					defaultOrigins := map[string]bool{
						"http://localhost:20001":  true,
						"http://127.0.0.1:20001": true,
					}
					if defaultOrigins[origin] {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
					}
				}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")

			// OPTIONS 预检请求直接返回
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware JWT 鉴权中间件
// 拦截 /api/* 路径，放行 /api/login 和 /api/health
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 放行登录接口和健康检查
	if path == "/api/login" || path == "/api/login/" || path == "/api/health" || path == "/api/health/" || strings.HasPrefix(path, "/api/proxy/image") {
		next.ServeHTTP(w, r)
		return
	}

		// 仅拦截 /api/* 路径
		if !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// 提取 Authorization 头
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"缺少 Authorization 头"}`, http.StatusUnauthorized)
			return
		}

		// 解析 Bearer Token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			http.Error(w, `{"error":"Authorization 格式错误，应为 Bearer <token>"}`, http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		claims, err := ValidateToken(tokenString)
		if err != nil {
			log.Printf("⚠️  JWT 校验失败: %v (来源: %s)", err, r.RemoteAddr)
			http.Error(w, `{"error":"Token 无效或已过期"}`, http.StatusUnauthorized)
			return
		}

		// 校验 Token 版本
		var user database.User
		if err := database.DB.Where("username = ? AND token_version = ?", claims.Username, claims.TokenVersion).First(&user).Error; err != nil {
			log.Printf("⚠️  Token 版本不匹配或用户已失效: %s", claims.Username)
			http.Error(w, `{"error":"登录已过期，请重新登录"}`, http.StatusUnauthorized)
			return
		}

		// 将 claims 注入 context 供后续 handler 使用
		ctx := context.WithValue(r.Context(), "claims", claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ProxyHeadersMiddleware 处理 Lucky 反向代理转发的头信息
// Lucky v2.27.2 通过 X-Forwarded-For / X-Forwarded-Proto / X-Forwarded-Host 传递原始请求信息
func ProxyHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 信任反代头
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			r.URL.Scheme = proto
		}
		if host := r.Header.Get("X-Forwarded-Host"); host != "" {
			r.Host = host
		}

		// 记录真实客户端 IP
		realIP := r.Header.Get("X-Forwarded-For")
		if realIP == "" {
			realIP = r.Header.Get("X-Real-IP")
		}
		if realIP != "" {
			r.RemoteAddr = realIP
		}

		next.ServeHTTP(w, r)
	})
}
