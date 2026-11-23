package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

func TestTemporaryUserStoreFailureDoesNotExpireSession(t *testing.T) {
	setupTestDB(t)
	if err := auth.InitSecret(); err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	u := database.User{Username: "owner", PasswordHash: hash}
	if err := database.DB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateToken("owner", u.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	newTestServer().registerRoutes(mux)
	handler := auth.AuthMiddleware(mux)
	call := func(method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}
	query := database.DB.Callback().Query()
	if err := query.Before("gorm:query").Register("test:account_store", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(errors.New("database is locked"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if got := call("GET", "/api/me", ""); got != 503 {
		t.Errorf("数据库暂不可用时会话状态 %d", got)
	}
	if got := call("GET", "/api/events/stream", ""); got != 503 {
		t.Errorf("数据库暂不可用时事件流状态 %d", got)
	}
	if got := call("POST", "/api/login", `{"username":"owner","password":"local-test-password"}`); got != 503 {
		t.Errorf("数据库暂不可用时登录状态 %d", got)
	}
	if err := query.Remove("test:account_store"); err != nil {
		t.Fatal(err)
	}
	if got := call("GET", "/api/me", ""); got != 200 {
		t.Fatalf("数据库恢复后原会话状态 %d", got)
	}
}

func TestConcurrentPasswordChangesCommitOneAccountVersion(t *testing.T) {
	setupTestDB(t)
	hash, err := auth.HashPassword("previous-password")
	if err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "owner", PasswordHash: hash}
	if err := database.DB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	read, release := make(chan struct{}, 2), make(chan struct{})
	callback := database.DB.Callback().Query()
	if err := callback.After("gorm:query").Register("test:simultaneous_changes", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			read <- struct{}{}
			<-release
		}
	}); err != nil {
		t.Fatal(err)
	}
	responses := make(chan int, 2)
	for _, body := range []string{`{"old_password":"previous-password","new_password":"replacement-one"}`, `{"old_password":"previous-password","new_password":"replacement-two"}`} {
		go func() {
			req := httptest.NewRequest("POST", "/api/user/change-password", strings.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), "claims", &auth.Claims{Username: user.Username, TokenVersion: user.TokenVersion}))
			w := httptest.NewRecorder()
			newTestServer().handleChangePassword(w, req)
			responses <- w.Code
		}()
	}
	<-read
	<-read
	close(release)
	got := map[int]int{}
	for range 2 {
		got[<-responses]++
	}
	if err := callback.Remove("test:simultaneous_changes"); err != nil {
		t.Fatal(err)
	}
	if got[200] != 1 || got[409] != 1 {
		t.Fatalf("从同一版本产生的两次密码修改 %v", got)
	}
}
