package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type filteredPresenceClient interface {
	TaskPresence(context.Context, []string) (map[string]bool, error)
}

func presenceClient(t *testing.T, client any) filteredPresenceClient {
	t.Helper()
	result, ok := client.(filteredPresenceClient)
	if !ok {
		t.Fatalf("%T 没有定向核对已绑定任务的入口", client)
	}
	return result
}

func TestFilteredTaskPresence(t *testing.T) {
	present := strings.Repeat("a", 40)
	missing := strings.Repeat("b", 40)
	v2 := strings.Repeat("c", 64)
	for _, proto := range []string{"qB", "Transmission"} {
		t.Run(proto, func(t *testing.T) {
			var calls int
			var query []string
			service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				calls++
				switch proto {
				case "qB":
					if r.Method != "GET" || r.URL.Path != "/api/v2/torrents/info" {
						t.Errorf("查询 %s %s", r.Method, r.URL)
					}
					query = strings.Split(r.URL.Query().Get("hashes"), "|")
					fmt.Fprintf(w, `[{"hash":%q}]`, strings.ToUpper(present))
				case "Transmission":
					var req struct {
						Method    string
						Arguments struct{ IDs, Fields []string }
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
					}
					if req.Method != "torrent-get" || !reflect.DeepEqual(req.Arguments.Fields, []string{"hashString"}) {
						t.Errorf("意外的查询 %+v", req)
					}
					query = req.Arguments.IDs
					fmt.Fprintf(w, `{"result":"success","arguments":{"torrents":[{"hashString":%q}]}}`, strings.ToUpper(present))
				}
			}))
			defer service.Close()
			var d any = NewQBittorrent(service.URL, "user", "pw", "")
			if proto == "Transmission" {
				d = NewTransmission(service.URL, "", "")
			}
			client := presenceClient(t, d)
			ids := []string{strings.ToUpper(present), missing, present, v2, "url:pending", "1234", ""}
			got, err := client.TaskPresence(context.Background(), ids)
			if err != nil || !reflect.DeepEqual(got, map[string]bool{present: true, missing: false}) {
				t.Fatalf("presence=%v err=%v", got, err)
			}
			if calls != 1 || !reflect.DeepEqual(query, []string{present, missing, v2}) {
				t.Fatalf("calls=%d query=%v", calls, query)
			}
			got, err = client.TaskPresence(context.Background(), []string{"url:unbound", ""})
			if err != nil || len(got) != 0 || calls != 1 {
				t.Fatalf("无绑定任务也查询下载器：%v %v calls=%d", got, err, calls)
			}
		})
	}
}

func TestFilteredTaskPresenceRejectsIncompleteReply(t *testing.T) {
	h := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name, proto, body string
		status            int
	}{
		{"null", "qB", "null", 200},
		{"blank", "qB", "", 200},
		{"missing hash", "qB", "[{}]", 200},
		{"invalid hash", "qB", `[{"hash":"stale"}]`, 200},
		{"bad json", "qB", "[", 200},
		{"trailing", "qB", "[] broken", 200},
		{"offline", "qB", "", 502},
		{"wrong tag", "Transmission", `{"result":"success","arguments":{"torrents":[]},"tag":-123}`, 200},
		{"trailing", "Transmission", `{"result":"success","arguments":{"torrents":[]}} broken`, 200},
		{"missing torrents", "Transmission", `{"result":"success","arguments":{}}`, 200},
		{"null torrents", "Transmission", `{"result":"success","arguments":{"torrents":null}}`, 200},
		{"missing hash", "Transmission", `{"result":"success","arguments":{"torrents":[{}]}}`, 200},
		{"bad result", "Transmission", `{"result":"failed","arguments":{"torrents":[]}}`, 200},
	} {
		t.Run(tc.proto+"/"+tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					w.WriteHeader(204)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer s.Close()
			var d any = NewQBittorrent(s.URL, "", "", "")
			if tc.proto == "Transmission" {
				d = NewTransmission(s.URL, "", "")
			}
			if got, err := presenceClient(t, d).TaskPresence(context.Background(), []string{h}); err == nil || len(got) != 0 {
				t.Fatalf("畸形响应被判定为下载器事实: %v %v", got, err)
			}
		})
	}
}

func TestQBPresenceRetriesAndKeepsFiltersBounded(t *testing.T) {
	var requests, logins int
	var sizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			logins++
			w.WriteHeader(204)
			return
		}
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		batch := strings.Split(r.URL.Query().Get("hashes"), "|")
		sizes = append(sizes, len(batch))
		if len(batch) > 48 || len(batch) == 0 {
			t.Errorf("单次核对任务数 %d", len(batch))
		}
		if len(sizes) == 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	var hashes []string
	for i := 1; i <= 120; i++ {
		hashes = append(hashes, fmt.Sprintf("%040x", i))
	}
	q := NewQBittorrent(server.URL, "", "", "")
	if got, err := q.TaskPresence(context.Background(), hashes); err == nil || len(got) != 0 {
		t.Fatalf("末页失败时返回了部分结论: %v %v", len(got), err)
	}
	if logins != 2 || !reflect.DeepEqual(sizes, []int{48, 48, 24}) {
		t.Fatalf("logins=%d batches=%v", logins, sizes)
	}
	got, err := q.TaskPresence(context.Background(), hashes[:1])
	if present, ok := got[hashes[0]]; err != nil || !ok || present {
		t.Fatalf("明确空清单: %v %v", got, err)
	}
}
