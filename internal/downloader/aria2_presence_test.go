package downloader

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAriaTaskPresenceRequiresCompleteProjection(t *testing.T) {
	const gid = "123456789abcdef0"
	missing := strings.Repeat("b", 40)
	found := strings.Repeat("a", 40)
	for _, mode := range []string{"complete", "full-page", "partial-error", "null-result", "malformed-member", "unidentified-task"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ID     int32
					Method string
					Params [][]struct {
						MethodName string
						Params     []any
					}
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Method != "system.multicall" || len(req.Params) != 1 {
					t.Errorf("unexpected request: %v %s", err, req.Method)
					return
				}
				var result []any
				for _, call := range req.Params[0] {
					if call.Params[0] != "token:fake-key" {
						t.Errorf("missing subcall token for %s", call.MethodName)
					}
					keys, ok := call.Params[len(call.Params)-1].([]any)
					if !ok || len(keys) == 0 {
						t.Errorf("expected projected fields")
					}
					sub := []map[string]string{}
					switch call.MethodName {
					case "aria2.tellActive":
						sub = append(sub, map[string]string{"gid": "0000000000000001", "infoHash": found, "status": "active"})
					case "aria2.tellWaiting", "aria2.tellStopped":
						if call.Params[1] != float64(0) {
							t.Errorf("unexpected offset")
						}
						limit := int(call.Params[2].(float64))
						if limit <= 0 || limit > 1000 {
							t.Errorf("unbounded limit %d", limit)
						}
						if call.MethodName == "aria2.tellWaiting" {
							switch mode {
							case "full-page":
								for len(sub) < limit {
									sub = append(sub, map[string]string{"gid": "0000000000000002", "status": "waiting"})
								}
							case "partial-error":
								result = append(result, map[string]any{"code": 1, "message": "unavailable"})
								continue
							case "null-result":
								result = append(result, []any{nil})
								continue
							case "unidentified-task":
								sub = append(sub, map[string]string{"gid": "0000000000000005", "status": "active"})
							case "malformed-member":
								sub = append(sub, map[string]string{})
							}
						} else {
							sub = append(sub, map[string]string{"gid": "0000000000000003", "infoHash": found, "status": "removed"})
						}
					case "aria2.tellStatus":
						id := call.Params[1].(string)
						if id != gid {
							t.Errorf("unexpected GID: %s", id)
						}
						result = append(result, map[string]any{"code": 1, "message": "GID " + id + " is not found"})
						continue
					default:
						t.Errorf("unexpected method %s", call.MethodName)
					}
					result = append(result, []any{sub})
				}
				json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "result": result})
			}))
			defer server.Close()
			got, err := NewAria2(server.URL, "fake-key").TaskPresence(t.Context(), []string{found, missing, gid})
			if err != nil || !got[found] {
				t.Fatalf("found task lost: %v %v", got, err)
			}
			if v, known := got[missing]; v || known != (mode == "complete") {
				t.Errorf("missing hash got %v, known=%v", v, known)
			}
			if v, known := got[gid]; v || !known {
				t.Errorf("missing GID got %v, known=%v", v, known)
			}
		})
	}
}

func TestAriaUnknownRPCReplyDoesNotProveAbsence(t *testing.T) {
	const id = "123456789abcdef0"
	for _, mode := range []string{"denied", "wrong-id", "trailing"} {
		t.Run(mode, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req aria2Req
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				msg := "GID " + id + " is not found"
				switch mode {
				case "denied":
					msg = "Unauthorized"
				case "wrong-id":
					req.ID++
				}
				json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "result": []any{map[string]any{"code": 1, "message": msg}}})
				if mode == "trailing" {
					w.Write([]byte("garbage"))
				}
			}))
			defer s.Close()
			got, err := NewAria2(s.URL, "").TaskPresence(t.Context(), []string{id})
			if len(got) != 0 || err == nil {
				t.Errorf("expected unknown, got %v, %v", got, err)
			}
		})
	}
}
