package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func TestDownloaderProbeSavedCredentials(t *testing.T) {
	setupTestDB(t)
	mux := http.NewServeMux()
	newTestServer().registerRoutes(mux)

	const user, password, secret = "test-user", "test-password", "test-token"
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			if r.FormValue("username") == user && r.FormValue("password") == password {
				http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-session", Path: "/"})
				w.Write([]byte("Ok."))
				return
			}
		case "/api/v2/app/version":
			if c, err := r.Cookie("SID"); err == nil && c.Value == "test-session" {
				w.Write([]byte("5.0.0"))
				return
			}
		case "/transmission/rpc":
			username, pass, ok := r.BasicAuth()
			if username == "other-user" && pass != "" {
				t.Error("edited username received a saved password")
			}
			if ok && username == user && pass == password {
				w.Write([]byte(`{"result":"success","arguments":{}}`))
				return
			}
		case "/jsonrpc":
			var body struct {
				ID     int
				Method string
				Params []string
			}
			if json.NewDecoder(r.Body).Decode(&body) == nil && body.Method == "aria2.getVersion" && len(body.Params) == 1 && body.Params[0] == "token:"+secret {
				json.NewEncoder(w).Encode(map[string]any{"id": body.ID, "jsonrpc": "2.0", "result": map[string]string{"version": "1.37.0"}})
				return
			}
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer rpc.Close()

	for _, key := range []string{"QB_USERNAME", "QB_PASSWORD", "TR_USERNAME", "TR_PASSWORD"} {
		t.Setenv(key, "")
	}
	for key, value := range map[string]string{
		"QB_HOST": rpc.URL, "QB_USER": user, "QB_PASS": password,
		"TR_HOST": rpc.URL, "TR_USER": user, "TR_PASS": password,
		"ARIA2_HOST": rpc.URL + "/jsonrpc", "ARIA2_SECRET": secret,
	} {
		t.Setenv(key, "")
		if err := database.DB.Save(&database.Setting{Key: key, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	probe := func(body map[string]any, want bool) {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/downloader/test", bytes.NewReader(raw)))
		var result struct{ Success bool }
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK || result.Success != want {
			t.Errorf("expected success=%v, got HTTP %d %s", want, w.Code, w.Body.String())
		}
	}
	for _, typ := range []string{"qbittorrent", "transmission", "aria2"} {
		host := rpc.URL
		identity := user
		if typ == "aria2" {
			host += "/jsonrpc"
			identity = ""
		}
		probe(map[string]any{"type": typ, "host": host, "username": identity, "use_saved_credentials": true}, true)
		probe(map[string]any{"type": typ, "host": host, "username": identity}, false)
	}
	// 修改主机地址或账号后，只能使用本次为该连接输入的凭据。
	other := httptest.NewServer(rpc.Config.Handler)
	defer other.Close()
	probe(map[string]any{"type": "aria2", "host": other.URL + "/jsonrpc", "use_saved_credentials": true}, false)
	probe(map[string]any{"type": "transmission", "host": rpc.URL, "username": "other-user", "use_saved_credentials": true}, false)
	probe(map[string]any{"type": "aria2", "host": rpc.URL + "/jsonrpc", "secret": "typed-wrong", "use_saved_credentials": true}, false)
}
