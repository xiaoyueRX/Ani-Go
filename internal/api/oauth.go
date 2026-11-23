package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

const bangumiStateCookie = "anigo_bangumi_flow"
const bangumiCallbackPath = "/api/bangumi/auth/callback"
const bangumiFlowLifetime = 10 * time.Minute

type bangumiFlow struct {
	state        string
	redirectURI  string
	tokenVersion int
	expires      time.Time
}

// 每个管理员只保留最近一次授权，浏览器凭证仅随回调路径发送。
type bangumiFlows struct {
	mu     sync.Mutex
	byUser map[string]bangumiFlow
}

func (f *bangumiFlows) start(w http.ResponseWriter, claims *auth.Claims, redirectURI string) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	state := hex.EncodeToString(raw[:])
	f.mu.Lock()
	if f.byUser == nil {
		f.byUser = make(map[string]bangumiFlow)
	}
	f.byUser[claims.Username] = bangumiFlow{state, redirectURI, claims.TokenVersion, time.Now().Add(bangumiFlowLifetime)}
	f.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: bangumiStateCookie, Value: state, Path: bangumiCallbackPath,
		HttpOnly: true, Secure: strings.HasPrefix(redirectURI, "https://"), SameSite: http.SameSiteLaxMode,
		MaxAge: int(bangumiFlowLifetime.Seconds()),
	})
	return state, nil
}

func (f *bangumiFlows) consume(r *http.Request, redirectURI string) bool {
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie(bangumiStateCookie)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		return false
	}
	f.mu.Lock()
	var username string
	var tokenVersion int
	for name, flow := range f.byUser {
		if time.Now().After(flow.expires) {
			delete(f.byUser, name)
			continue
		}
		if flow.redirectURI == redirectURI && subtle.ConstantTimeCompare([]byte(flow.state), []byte(state)) == 1 {
			username, tokenVersion = name, flow.tokenVersion
			delete(f.byUser, name)
			break
		}
	}
	f.mu.Unlock()
	if username == "" {
		return false
	}
	var count int64
	err = database.DB.WithContext(r.Context()).Model(&database.User{}).Where("username = ? AND token_version = ?", username, tokenVersion).Count(&count).Error
	return err == nil && count == 1
}
