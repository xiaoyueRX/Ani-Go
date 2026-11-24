package downloader

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestParseQBittorrentList_EmptyAndEOF(t *testing.T) {
	// Test empty reader (EOF)
	tasks, err := parseQBittorrentList(strings.NewReader(""))
	if err != nil {
		t.Fatalf("parse empty reader failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}

	// Test empty json array
	tasks, err = parseQBittorrentList(strings.NewReader("[]"))
	if err != nil {
		t.Fatalf("parse empty array failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}

	// Test valid tasks
	jsonStr := `[{"hash":"abc","name":"test","save_path":"/tmp","content_path":"/tmp/test.mkv","state":"downloading","progress":0.5,"dlspeed":1024,"upspeed":512,"size":10000,"completed":5000,"uploaded":2000,"ratio":0.4,"seeding_time":100}]`
	tasks, err = parseQBittorrentList(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("parse valid tasks failed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Hash != "abc" || tasks[0].Progress != 0.5 {
		t.Fatalf("unexpected tasks result: %+v", tasks)
	}
}

func TestQBittorrent_ConcurrentLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-session-id"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/v2/torrents/info" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]qbTorrentInfo{
				{Hash: "123", Name: "Demo"},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	qb := NewQBittorrent(server.URL, "admin", "admin", "anime")

	// Concurrently invoke ensureLogin and List
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = qb.ensureLogin(ctx)
			tasks, err := qb.List(ctx)
			if err != nil || len(tasks) != 1 {
				t.Errorf("concurrent List failed: %v, tasks=%d", err, len(tasks))
			}
		}()
	}
	wg.Wait()
}

func TestQBittorrentAvailabilityAfterRestart(t *testing.T) {
	for _, rejected := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(rejected), func(t *testing.T) {
			// 真实超时保证缺少探测时处理器也能退出。
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			deadline := ctx.Done()
			// 监听回环套接字期间虚拟时钟不前进，两次登录会取得相同时间。
			synctest.Test(t, func(t *testing.T) {
				var restarted atomic.Bool
				var logins, staleProbes atomic.Int32
				bothStale := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					session := "original"
					if restarted.Load() {
						session = "restarted"
					}
					switch r.URL.Path {
					case "/api/v2/auth/login":
						logins.Add(1)
						http.SetCookie(w, &http.Cookie{Name: "SID", Value: session, Path: "/"})
						_, _ = w.Write([]byte("Ok."))
					case "/api/v2/app/version":
						sid, err := r.Cookie("SID")
						if err != nil || sid.Value != session {
							if restarted.Load() {
								if staleProbes.Add(1) == 2 {
									close(bothStale)
								}
								select {
								case <-bothStale:
								case <-r.Context().Done():
								case <-deadline:
									t.Error("等待两个旧会话探测超时")
								}
							}
							w.WriteHeader(rejected)
						}
					default:
						http.NotFound(w, r)
					}
				}))
				defer server.Close()
				qb := NewQBittorrent(server.URL, "admin", "pass", "")
				if !qb.IsAvailable(t.Context()) {
					t.Fatal("首次连接失败")
				}
				restarted.Store(true)
				results := make(chan bool, 2)
				for range 2 {
					go func() { results <- qb.IsAvailable(t.Context()) }()
				}
				first, second := <-results, <-results
				if !first || !second || !qb.IsAvailable(t.Context()) || logins.Load() != 2 {
					t.Fatalf("重启后应重新登录一次并复用新会话，实际共登录 %d 次", logins.Load())
				}
				server.Close()
				if qb.IsAvailable(t.Context()) {
					t.Fatal("连接已关闭的客户端应当离线")
				}
			})
		})
	}
}

func TestQBittorrentAvailabilityDenied(t *testing.T) {
	for _, test := range []struct {
		name        string
		loginBody   string
		versionCode int
		wantLogins  int32
		wantProbes  int32
	}{
		{"错误密码", "Fails.", http.StatusForbidden, 1, 0},
		{"权限不足", "Ok.", http.StatusForbidden, 2, 2},
		{"服务故障", "Ok.", http.StatusServiceUnavailable, 1, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logins, probes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/auth/login":
					logins.Add(1)
					_, _ = w.Write([]byte(test.loginBody))
				case "/api/v2/app/version":
					probes.Add(1)
					w.WriteHeader(test.versionCode)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			qb := NewQBittorrent(server.URL, "admin", "password", "")
			if qb.IsAvailable(t.Context()) || logins.Load() != test.wantLogins || probes.Load() != test.wantProbes {
				t.Fatalf("连接应失败并停止重试，实际登录 %d 次，探测 %d 次", logins.Load(), probes.Load())
			}
		})
	}
}

func TestAria2_DeleteWithFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aria2_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dummyFile := filepath.Join(tmpDir, "episode.mkv")
	if err := os.WriteFile(dummyFile, []byte("video data"), 0644); err != nil {
		t.Fatal(err)
	}
	aria2Meta := dummyFile + ".aria2"
	if err := os.WriteFile(aria2Meta, []byte("control"), 0644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req aria2Req
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.Method == "aria2.tellStatus" {
			resp := aria2Resp{
				JSONRPC: "2.0",
				ID:      req.ID,
			}
			st := aria2Status{
				GID: "gid-123",
				Files: []struct {
					Path string `json:"path"`
				}{
					{Path: dummyFile},
				},
			}
			raw, _ := json.Marshal(st)
			resp.Result = raw
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// simulate remove failing because download stopped
		if req.Method == "aria2.remove" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error": map[string]interface{}{
					"code":    1,
					"message": "Cannot remove stopped download",
				},
			})
			return
		}

		// fallback to removeDownloadResult
		if req.Method == "aria2.removeDownloadResult" {
			resp := aria2Resp{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`"OK"`),
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewAria2(server.URL, "token")
	ctx := context.Background()
	err = client.Delete(ctx, "gid-123", true)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify local files deleted
	if _, err := os.Stat(dummyFile); !os.IsNotExist(err) {
		t.Errorf("expected dummyFile to be removed")
	}
	if _, err := os.Stat(aria2Meta); !os.IsNotExist(err) {
		t.Errorf("expected aria2Meta to be removed")
	}
}

func TestDownloader_InputValidationAndEmptyGuards(t *testing.T) {
	ctx := context.Background()

	// 1. QBittorrent 空 host 与空 hash 校验
	qbEmpty := NewQBittorrent("", "admin", "admin", "anime")
	if qbEmpty.IsAvailable(ctx) {
		t.Error("空 host 的 QBittorrent.IsAvailable 应返回 false")
	}
	if _, err := qbEmpty.GetStatus(ctx, ""); err == nil {
		t.Error("空 hash 的 QBittorrent.GetStatus 应报错")
	}
	if err := qbEmpty.Delete(ctx, "", false); err == nil {
		t.Error("空 hash 的 QBittorrent.Delete 应报错")
	}
	if err := qbEmpty.Pause(ctx, ""); err == nil {
		t.Error("空 hash 的 QBittorrent.Pause 应报错")
	}
	if err := qbEmpty.Resume(ctx, ""); err == nil {
		t.Error("空 hash 的 QBittorrent.Resume 应报错")
	}

	// 2. Transmission 空 host 与空 target 校验
	trEmpty := NewTransmission("", "user", "pass")
	if trEmpty.IsAvailable(ctx) {
		t.Error("空 host 的 Transmission.IsAvailable 应返回 false")
	}
	if _, err := trEmpty.GetStatus(ctx, ""); err == nil {
		t.Error("空 hash 的 Transmission.GetStatus 应报错")
	}
	if err := trEmpty.Delete(ctx, "", false); err == nil {
		t.Error("空 hash 的 Transmission.Delete 应报错")
	}
	if err := trEmpty.Pause(ctx, ""); err == nil {
		t.Error("空 hash 的 Transmission.Pause 应报错")
	}
	if err := trEmpty.Resume(ctx, ""); err == nil {
		t.Error("空 hash 的 Transmission.Resume 应报错")
	}

	// 3. Aria2 空 host 与空 target 校验
	arEmpty := NewAria2("", "token")
	if arEmpty.IsAvailable(ctx) {
		t.Error("空 host 的 Aria2.IsAvailable 应返回 false")
	}
	if _, err := arEmpty.GetStatus(ctx, ""); err == nil {
		t.Error("空 target 的 Aria2.GetStatus 应报错")
	}
	if err := arEmpty.Delete(ctx, "", false); err == nil {
		t.Error("空 target 的 Aria2.Delete 应报错")
	}
	if err := arEmpty.Pause(ctx, ""); err == nil {
		t.Error("空 target 的 Aria2.Pause 应报错")
	}
	if err := arEmpty.Resume(ctx, ""); err == nil {
		t.Error("空 target 的 Aria2.Resume 应报错")
	}
}

func TestConvertAria2Status_BittorrentInfoName(t *testing.T) {
	st := aria2Status{
		GID:    "1234567890abcdef",
		Status: "active",
		Bittorrent: &struct {
			InfoHash string `json:"infoHash"`
			Info     *struct {
				Name string `json:"name"`
			} `json:"info"`
		}{
			InfoHash: "abc123hash",
			Info: &struct {
				Name string `json:"name"`
			}{
				Name: "[SubGroup] Anime Title S01E01 [1080p]",
			},
		},
	}

	task := convertAria2Status(st)
	if task.Name != "[SubGroup] Anime Title S01E01 [1080p]" {
		t.Errorf("期望从 Bittorrent.Info.Name 提取任务名, 实际得到: %q", task.Name)
	}
	if task.Hash != "abc123hash" {
		t.Errorf("期望 Hash 为 'abc123hash', 实际得到: %q", task.Hash)
	}
}

func TestQBittorrentOperationsRecoverAfterRestart(t *testing.T) {
	runCase := func(t *testing.T, op func(t *testing.T, qb *QBittorrent, tmpDir string)) {
		t.Helper()
		var sessionSeq atomic.Int32
		sessionSeq.Store(1)
		var logins atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/auth/login" {
				n := logins.Add(1)
				sid := "v1"
				if sessionSeq.Load() > 1 {
					sid = "v2"
				}
				_ = n
				http.SetCookie(w, &http.Cookie{Name: "SID", Value: sid, Path: "/"})
				_, _ = w.Write([]byte("Ok."))
				return
			}
			wantSID := "v1"
			if sessionSeq.Load() > 1 {
				wantSID = "v2"
			}
			sid, err := r.Cookie("SID")
			if err != nil || sid.Value != wantSID {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			switch r.URL.Path {
			case "/api/v2/app/version":
				_, _ = w.Write([]byte("v4.6.3"))
			case "/api/v2/torrents/info":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"hash":"deadbeef01","name":"Demo","save_path":"/dl","state":"uploading","progress":1,"size":10,"amount_left":0,"completed":10}]`))
			case "/api/v2/torrents/files":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"name":"Demo.mkv","size":10,"priority":1,"progress":1}]`))
			case "/api/v2/torrents/stop", "/api/v2/torrents/start":
				w.WriteHeader(http.StatusNotFound)
			case "/api/v2/torrents/pause", "/api/v2/torrents/resume", "/api/v2/torrents/addTags", "/api/v2/torrents/delete":
				w.WriteHeader(http.StatusOK)
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		qb := NewQBittorrent(server.URL, "admin", "pass", "ani-go")
		if !qb.IsAvailable(t.Context()) {
			t.Fatal("首次建立会话失败")
		}
		if logins.Load() != 1 {
			t.Fatalf("期望首次登录 1 次，实际 %d 次", logins.Load())
		}
		// 模拟 qBittorrent 重启导致原 SID=v1 失效
		sessionSeq.Store(2)

		op(t, qb, t.TempDir())
		if logins.Load() != 2 {
			t.Fatalf("期望遇 403 后自动重登为 2 次登录，实际 %d 次", logins.Load())
		}
	}

	t.Run("List", func(t *testing.T) {
		runCase(t, func(t *testing.T, qb *QBittorrent, _ string) {
			tasks, err := qb.List(t.Context())
			if err != nil || len(tasks) != 1 || tasks[0].Hash != "deadbeef01" {
				t.Fatalf("List 恢复失败: tasks=%+v err=%v", tasks, err)
			}
		})
	})

	t.Run("GetStatus", func(t *testing.T) {
		runCase(t, func(t *testing.T, qb *QBittorrent, _ string) {
			task, err := qb.GetStatus(t.Context(), "deadbeef01")
			if err != nil || task.Hash != "deadbeef01" {
				t.Fatalf("GetStatus 恢复失败: task=%+v err=%v", task, err)
			}
		})
	})

	t.Run("CompletedFiles", func(t *testing.T) {
		runCase(t, func(t *testing.T, qb *QBittorrent, tmpDir string) {
			sel, err := qb.CompletedFiles(t.Context(), core.DownloadTask{Hash: "deadbeef01", SavePath: tmpDir})
			if err != nil || len(sel.Files) != 1 {
				t.Fatalf("CompletedFiles 恢复失败: sel=%+v err=%v", sel, err)
			}
		})
	})

	t.Run("Pause_Fallback4x", func(t *testing.T) {
		runCase(t, func(t *testing.T, qb *QBittorrent, _ string) {
			if err := qb.Pause(t.Context(), "deadbeef01"); err != nil {
				t.Fatalf("Pause 恢复与 404 回退失败: %v", err)
			}
		})
	})

	t.Run("Resume_Fallback4x", func(t *testing.T) {
		runCase(t, func(t *testing.T, qb *QBittorrent, _ string) {
			if err := qb.Resume(t.Context(), "deadbeef01"); err != nil {
				t.Fatalf("Resume 恢复与 404 回退失败: %v", err)
			}
		})
	})

	t.Run("AddTags", func(t *testing.T) {
		runCase(t, func(t *testing.T, qb *QBittorrent, _ string) {
			if err := qb.AddTags(t.Context(), "deadbeef01", "ani-go"); err != nil {
				t.Fatalf("AddTags 恢复失败: %v", err)
			}
		})
	})

	t.Run("Delete", func(t *testing.T) {
		runCase(t, func(t *testing.T, qb *QBittorrent, _ string) {
			if err := qb.Delete(t.Context(), "deadbeef01", false); err != nil {
				t.Fatalf("Delete 恢复失败: %v", err)
			}
		})
	})
}
