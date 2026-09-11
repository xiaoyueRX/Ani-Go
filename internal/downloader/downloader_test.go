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
	"testing"
	"time"
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
