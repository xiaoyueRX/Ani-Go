package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/api"
	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

func TestMarkDownloadingEpisodeStillReachesLibrary(t *testing.T) {
	db := pipelineDB(t)
	if err := db.AutoMigrate(&database.User{}); err != nil {
		t.Fatal(err)
	}
	if err := auth.InitSecretFromDB(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "tester", PasswordHash: "unused"}
	savePipelineRow(t, db, &user)
	token, err := auth.GenerateToken(user.Username, user.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}

	sub := database.Subscription{TitleCN: "Series", Season: 1, AnimeType: "tv", TotalEpisodes: 1}
	savePipelineRow(t, db, &sub)
	ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, Status: "downloading", TorrentHash: "hash"}
	savePipelineRow(t, db, &ep)
	src := filepath.Join(t.TempDir(), "source.mkv")
	const content = "episode payload"
	if err := os.WriteFile(src, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	dl := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{tasks: []core.DownloadTask{{Hash: ep.TorrentHash, ContentPath: src, Status: "downloading", Progress: 0.4}}}}
	org := organizer.New("Series/E01{ext}", "", "", t.TempDir(), "", true, nil)
	s := &Scheduler{downloader: dl, organizer: org}
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	srv := api.StartServer(ctx, "127.0.0.1", 0, "test", nil, dl, nil, nil, nil, nil, nil, nil, nil, "", nil, nil, nil, api.ServerOptions{Stopped: stopped})
	t.Cleanup(func() { cancel(); <-stopped })
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/episodes/%d/status", ep.ID), strings.NewReader(`{"status":"completed"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	reply := httptest.NewRecorder()
	srv.Handler.ServeHTTP(reply, req)
	if reply.Code != http.StatusOK {
		t.Fatalf("手工标记 HTTP %d %s", reply.Code, reply.Body.String())
	}

	s.pollDownloads(ctx)
	s.pollOrganizer(ctx)
	if err := db.First(&ep, ep.ID).Error; err != nil || ep.Status != "downloading" || ep.ManualStatus != "completed" || ep.FinalPath != "" {
		t.Fatalf("未完成的文件提前整理：%+v %v", ep, err)
	}
	if err := db.First(&sub, sub.ID).Error; err != nil || sub.CurrentEpisodes != 0 || sub.Completed {
		t.Fatalf("未完成订阅的实际进度：%+v %v", sub, err)
	}

	dl.tasks[0].Progress, dl.tasks[0].Status = 1, "completed"
	s.pollDownloads(ctx)
	s.pollOrganizer(ctx)
	if err := db.First(&ep, ep.ID).Error; err != nil || ep.Status != "organized" || ep.ManualStatus != "completed" || ep.DownloadFinishedAt == nil || ep.OrganizedAt == nil {
		t.Fatalf("下载完成后手工标记仍应随整理结果保留：%+v %v", ep, err)
	}
	data, err := os.ReadFile(ep.FinalPath)
	if err != nil || string(data) != content {
		t.Fatalf("整理文件内容 %q %v", data, err)
	}
	if err := db.First(&sub, sub.ID).Error; err != nil || sub.CurrentEpisodes != 1 {
		t.Fatalf("已入库进度：%+v %v", sub, err)
	}
}
