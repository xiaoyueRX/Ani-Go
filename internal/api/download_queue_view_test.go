package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type disconnectedQueue struct{ stubDownloader }

func (*disconnectedQueue) List(context.Context) ([]core.DownloadTask, error) {
	return nil, errors.New("downloader offline")
}

func TestDownloadQueueReportsDisconnectedDownloader(t *testing.T) {
	w := httptest.NewRecorder()
	(&Server{downloader: &disconnectedQueue{}}).handleListDownloads(w, httptest.NewRequest(http.MethodGet, "/api/downloads", nil))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("disconnected downloader returned HTTP %d with %s", w.Code, w.Body.String())
	}
}

func TestDownloadQueueReturnsDisplayStateAndAvailableControls(t *testing.T) {
	tasks := []core.DownloadTask{{Status: "stopped", Progress: .4}, {Status: "stoppedUP", Progress: 1}, {Status: "uploading", Progress: 1}, {Status: "active", Progress: .32}, {Status: "queuedDL"}, {Status: "error"}, {Status: "completed", Progress: 1}}
	w := httptest.NewRecorder()
	(&Server{downloader: &stubDownloader{tasks: tasks}}).handleListDownloads(w, httptest.NewRequest(http.MethodGet, "/api/downloads", nil))
	var got []struct {
		Status   string `json:"status"`
		Display  string `json:"display_status"`
		Complete bool   `json:"complete"`
		Pause    bool   `json:"can_pause"`
		Resume   bool   `json:"can_resume"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got) != len(tasks) {
		t.Fatal("invalid queue", err)
	}
	views := []string{"paused", "paused", "seeding", "downloading", "queued", "error", "completed"}
	for i, row := range got {
		if row.Status != tasks[i].Status || row.Display != views[i] || row.Pause != (i == 2 || i == 3 || i == 4) || row.Resume != (i == 0 || i == 1) || row.Complete != (i == 1 || i == 2 || i == 6) {
			t.Fatalf("wrong capabilities for %s: %+v", tasks[i].Status, row)
		}
	}
}
