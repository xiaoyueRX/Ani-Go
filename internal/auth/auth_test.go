package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("admin123")
	if err != nil {
		t.Fatalf("哈希密码失败: %v", err)
	}
	if hash == "admin123" {
		t.Fatal("密码明文未被哈希")
	}
	if !CheckPassword("admin123", hash) {
		t.Fatal("正确密码校验失败")
	}
	if CheckPassword("wrong", hash) {
		t.Fatal("错误密码不应通过校验")
	}
}

func TestDynamicSecretInit(t *testing.T) {
	if err := InitSecret(); err != nil {
		t.Fatalf("Secret 初始化失败: %v", err)
	}
	if len(jwtSecret) != 32 {
		t.Fatalf("Secret 长度 = %d, 期望 32", len(jwtSecret))
	}
}

func TestGenerateAndValidateToken(t *testing.T) {
	InitSecret()
	token, err := GenerateToken("testuser")
	if err != nil {
		t.Fatalf("Token 生成失败: %v", err)
	}
	if token == "" {
		t.Fatal("Token 为空")
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("Token 校验失败: %v", err)
	}
	if claims.Username != "testuser" {
		t.Fatalf("用户名 = %q, 期望 testuser", claims.Username)
	}
}

func TestValidateInvalidToken(t *testing.T) {
	InitSecret()
	// 未签名的伪造 token
	_, err := ValidateToken("invalid.token.here")
	if err == nil {
		t.Fatal("无效 Token 不应通过校验")
	}
}

func TestValidateWrongSecret(t *testing.T) {
	InitSecret()
	token, _ := GenerateToken("user")

	// 换一个新 Secret 模拟重启后旧 Token 失效
	InitSecret()
	_, err := ValidateToken(token)
	if err == nil {
		t.Fatal("旧 Secret 签发的 Token 不应通过新 Secret 的校验")
	}
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/subscriptions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 Token 请求应返回 401，实际: %d", rec.Code)
	}
}

func TestAuthMiddleware_BypassLogin(t *testing.T) {
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatal("登录接口不应被拦截")
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	InitSecret()
	token, _ := GenerateToken("admin")

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("有效 Token 请求应返回 200，实际: %d", rec.Code)
	}
}

func TestCORSMiddleware_Options(t *testing.T) {
	handler := CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/api/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("OPTIONS 预检应返回 204，实际: %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS Allow-Origin 应设为 *")
	}
}
