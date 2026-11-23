package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestAriaLostAddReceiptDoesNotCreateAnotherTask(t *testing.T) {
	t.Setenv("DISK_MIN_FREE_GB", "0")
	const uri = "https://example.org/episode.torrent"
	var mu sync.Mutex
	active := map[string]bool{}
	adds := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req aria2Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch req.Method {
		case "aria2.addUri":
			adds++
			gid := fmt.Sprintf("%016x", adds)
			if len(req.Params) > 1 {
				if options, ok := req.Params[1].(map[string]any); ok {
					if id, ok := options["gid"].(string); ok {
						gid = id
					}
				}
			}
			if active[gid] {
				fmt.Fprintf(w, `{"id":%d,"error":{"code":1,"message":"GID is not unique"}}`, req.ID)
				return
			}
			active[gid] = true
			if adds == 1 {
				http.Error(w, "upstream response lost", http.StatusBadGateway)
				return
			}
			fmt.Fprintf(w, `{"id":%d,"result":"%s"}`, req.ID, gid)
		case "aria2.tellStatus":
			id := req.Params[0].(string)
			if !active[id] {
				fmt.Fprintf(w, `{"id":%d,"error":{"code":1,"message":"not found"}}`, req.ID)
				return
			}
			fmt.Fprintf(w, `{"id":%d,"result":{"gid":"%s","status":"waiting","files":[{"uris":[{"uri":%q}]}]}}`, req.ID, id, uri)
		default:
			t.Errorf("unexpected RPC %s", req.Method)
		}
	}))
	defer server.Close()
	item := core.TorrentItem{URL: uri, TrackingTag: "anigo-id-stable"}
	first, err := NewAria2(server.URL, "").AddWithID(context.Background(), item, "")
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := NewAria2(server.URL, "").AddWithID(context.Background(), item, "")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if first != recovered || len(first) != 16 || len(active) != 1 {
		t.Fatalf("repeated accepted jobs: first %q recovered %q count %d", first, recovered, len(active))
	}
}
