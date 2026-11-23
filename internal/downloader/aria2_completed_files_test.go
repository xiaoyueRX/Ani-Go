package downloader

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func archivedAria2Client(t *testing.T, status map[string]any) *Aria2 {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req aria2Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if req.Method != "aria2.tellStatus" || len(req.Params) != 2 || req.Params[0] != status["gid"] {
			t.Errorf("unexpected RPC: %+v", req)
			return
		}
		keys, ok := req.Params[1].([]any)
		if !ok {
			t.Error("missing status keys")
			return
		}
		result := make(map[string]any)
		for _, key := range keys {
			name, _ := key.(string)
			if value, exists := status[name]; exists {
				result[name] = value
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	t.Cleanup(server.Close)
	return NewAria2(server.URL, "")
}

func TestAria2ArchivedSelectedFiles(t *testing.T) {
	const gid = "0123456789abcdef"
	for _, tc := range []struct {
		name, status, total, done, perFile string
		accept                             bool
	}{
		{"completed and archived", "complete", "8", "8", "0", true},
		{"active", "active", "8", "8", "0", false},
		{"paused", "paused", "8", "8", "0", false},
		{"error", "error", "8", "8", "0", false},
		{"selected total differs", "complete", "12", "12", "0", false},
		{"selected bytes incomplete", "complete", "8", "7", "0", false},
		{"aggregate omitted", "complete", "", "", "0", false},
		{"partial file counter", "complete", "8", "8", "3", false},
		{"missing file counter", "complete", "8", "8", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			files := make([]map[string]string, 0, 3)
			for i, name := range []string{"S01E01.mkv", "S01E02.mkv", "S01E03.mkv"} {
				f := map[string]string{"path": filepath.Join(root, name), "length": "4", "selected": "true"}
				if i == 1 {
					f["selected"] = "false"
				}
				if tc.perFile != "" {
					f["completedLength"] = tc.perFile
				}
				files = append(files, f)
			}
			status := map[string]any{"gid": gid, "status": tc.status, "files": files}
			if tc.total != "" {
				status["totalLength"], status["completedLength"] = tc.total, tc.done
			}
			client := archivedAria2Client(t, status)
			task := core.DownloadTask{ClientID: gid, SavePath: root}
			list, err := client.CompletedFiles(t.Context(), task)
			if !tc.accept {
				if err == nil {
					t.Fatalf("accepted unverified archived files: %+v", list)
				}
				return
			}
			if err != nil || !list.Partial || len(list.Files) != 2 ||
				list.Files[0].Path != files[0]["path"] || list.Files[1].Path != files[2]["path"] ||
				list.Files[0].Size != 4 || list.Files[1].Size != 4 {
				t.Fatalf("archived selection = %+v, err = %v", list, err)
			}
		})
	}
}

func TestAria2ArchivedFilesSharingPieces(t *testing.T) {
	for _, tc := range []struct {
		name, bitfield, total, middleIndex, middleLength string
		accept                                           bool
	}{
		{"first and last files share boundary pieces with middle", "b0", "10", "2", "7", true},
		{"selections share the same piece", "c0", "7", "2", "1", true},
		{"selected piece missing", "a0", "10", "2", "7", false},
		{"missing bitfield", "", "10", "2", "7", false},
		{"missing bitfield contradicts shared geometry", "", "7", "2", "7", false},
		{"no bitfield but exactly aligned geometry", "", "8", "2", "4", true},
		{"truncated bitfield", "b", "10", "2", "7", false},
		{"reported download length differs from selected pieces", "b0", "12", "2", "7", false},
		{"unknown file order", "b0", "10", "", "7", false},
		{"file index out of order", "b0", "10", "3", "7", false},
		{"overflowing offsets", "b0", "10", "2", "9223372036854775807", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const gid = "1123456789abcdef"
			root := t.TempDir()
			files := []map[string]string{
				{"index": "1", "length": "3", "path": filepath.Join(root, "a.mkv"), "completedLength": "0", "selected": "true"},
				{"index": tc.middleIndex, "length": tc.middleLength, "path": filepath.Join(root, "b.mkv"), "completedLength": "0", "selected": "false"},
				{"index": "3", "length": "4", "path": filepath.Join(root, "c.mkv"), "completedLength": "0", "selected": "true"},
			}
			if tc.middleIndex == "" {
				delete(files[1], "index")
			}
			pieces := "4"
			if tc.middleLength == "4" {
				pieces = "3"
				files[0]["length"] = "4"
			}
			if tc.middleLength == "1" {
				pieces = "2"
				files[0]["length"] = "2"
			}
			status := map[string]any{
				"gid": gid, "files": files, "status": "complete",
				"totalLength": tc.total, "completedLength": tc.total,
				"bitfield": tc.bitfield, "numPieces": pieces, "pieceLength": "4",
			}
			client := archivedAria2Client(t, status)
			list, err := client.CompletedFiles(t.Context(), core.DownloadTask{SavePath: root, ClientID: gid})
			if tc.accept {
				if err != nil || !list.Partial || len(list.Files) != 2 || list.Files[0].Path != files[0]["path"] || list.Files[1].Path != files[2]["path"] {
					t.Fatalf("shared piece selection = %+v, err = %v", list, err)
				}
			} else if err == nil {
				t.Fatalf("accepted inconsistent piece coverage: %+v", list)
			}
		})
	}
}
