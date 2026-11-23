package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestAriaSelectedFilesRequireCompletedLocalFile(t *testing.T) {
	var completed = "3"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req aria2Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Method != "aria2.tellStatus" || len(req.Params) != 2 || req.Params[0] != "gid" {
			t.Errorf("unexpected RPC %+v %v", req, err)
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"gid":"gid","status":"active","files":[{"path":"/download/Show/E01.mkv","length":"3","completedLength":"%s","selected":"true"},{"path":"/download/Show/E02.mkv","length":"600","completedLength":"0","selected":"false"}]}}`, req.ID, completed)
	}))
	defer server.Close()
	client := NewAria2(server.URL, "")
	task := core.DownloadTask{SavePath: "/download", ClientID: "gid"}
	list, err := client.CompletedFiles(context.Background(), task)
	if err != nil || !list.Partial || len(list.Files) != 1 || list.Files[0].Size != 3 {
		t.Fatalf("selection %+v %v", list, err)
	}
	completed = "2"
	if _, err := client.CompletedFiles(context.Background(), task); err == nil {
		t.Fatal("unfinished selected file accepted")
	}
}

func TestTransmissionIncompleteSelectionIsNotAnEmptyQueue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":"success","arguments":{"torrents":[{"hashString":"abcd","files":[{"name":"one.mkv","length":30,"bytesCompleted":30}],"fileStats":[]}]}}`)
	}))
	defer server.Close()
	client := NewTransmission(server.URL, "", "")
	if _, err := client.CompletedFiles(context.Background(), core.DownloadTask{Hash: "abcd", SavePath: "/download"}); err == nil {
		t.Fatal("missing wanted flags accepted")
	}
}

func TestReportedSingleVideo(t *testing.T) {
	for _, tc := range []struct {
		name, other string
		selected    bool
		single      bool
	}{
		{"unselected version", "Alternative.M4V", false, false},
		{"unselected subtitle", "Movie.zh.ASS", false, true},
		{"selected subtitle", "Movie.en.srt", true, true},
		{"selected version", "Alternate.webm", true, false},
		{"selected unsupported video", "Trailer.wmv", true, false},
		{"selected unknown asset", "part.bin", true, false},
		{"unknown unselected file", "part.bin", false, false},
		{"unnamed unselected file", "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := core.DownloadTask{SavePath: t.TempDir(), Hash: "abcd", ClientID: "gid"}
			selected := 0
			if tc.selected {
				selected = 1
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/auth/login":
					fmt.Fprint(w, "Ok.")
				case "/api/v2/torrents/files":
					fmt.Fprintf(w, `[{"name":"Movie.mkv","size":3,"progress":1,"priority":1},{"name":%q,"size":2,"progress":1,"priority":%d},{"name":"Info.nfo","size":1,"priority":0}]`, tc.other, selected)
				case "/transmission/rpc":
					fmt.Fprintf(w, `{"result":"success","arguments":{"torrents":[{"hashString":"abcd","files":[{"name":"Movie.mkv","length":3,"bytesCompleted":3},{"name":%q,"length":2,"bytesCompleted":2},{"name":"Info.nfo","length":1}],"fileStats":[{"wanted":true},{"wanted":%t},{"wanted":false}]}]}}`, tc.other, tc.selected)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			// aria2 给出绝对路径；空路径保持缺失，不用根目录来代替文件名。
			otherPath := tc.other
			if otherPath != "" {
				otherPath = filepath.Join(task.SavePath, otherPath)
			}
			aria := archivedAria2Client(t, map[string]any{"gid": "gid", "status": "active", "files": []map[string]any{
				{"path": filepath.Join(task.SavePath, "Movie.mkv"), "length": "3", "completedLength": "3", "selected": "true"},
				{"path": otherPath, "length": "2", "completedLength": "2", "selected": strconv.FormatBool(tc.selected)},
				{"path": filepath.Join(task.SavePath, "Info.nfo"), "length": "1", "selected": "false"},
			}})
			for _, client := range []FileLister{NewQBittorrent(server.URL, "admin", "pass", ""), NewTransmission(server.URL, "", ""), aria} {
				files, err := client.CompletedFiles(t.Context(), task)
				if err != nil || files.IsSingleVideo() != tc.single {
					t.Errorf("%T: IsSingleVideo = %v, want %v, err = %v", client, files.IsSingleVideo(), tc.single, err)
				}
			}
		})
	}
}

func TestSingleVideoWithUnknownSelection(t *testing.T) {
	selection := FileSelection{Partial: true, Files: []CompletedFile{{Path: "OnlySelected.mkv", Size: 1}}}
	if selection.IsSingleVideo() {
		t.Fatal("无法从部分清单推断整个任务只有一个视频")
	}
	selection.Partial = false
	if !selection.IsSingleVideo() {
		t.Fatal("完整清单中只有一个视频")
	}
}
