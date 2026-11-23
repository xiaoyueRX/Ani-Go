package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
