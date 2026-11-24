package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
)

type presenceStub struct {
	stubDownloader
	ids   []string
	found map[string]bool
	err   error
}

func (s *presenceStub) TaskPresence(_ context.Context, ids []string) (map[string]bool, error) {
	s.ids = slices.Clone(ids)
	return s.found, s.err
}

func TestSubscriptionChecksBoundTasksWithoutChangingHistory(t *testing.T) {
	setupTestDB(t)
	sub := seedSubscription(t, database.Subscription{TitleCN: "恢复测试"})
	other := seedSubscription(t, database.Subscription{TitleCN: "其他剧集"})
	episodes := []database.Episode{
		{SubscriptionID: sub.ID, Status: "downloading", TorrentHash: "ABC"},
		{SubscriptionID: sub.ID, Status: "downloaded", TorrentHash: "abc"},
		{SubscriptionID: sub.ID, Status: "downloading", TorrentHash: "Def"},
		{SubscriptionID: sub.ID, Status: "downloaded", TorrentHash: "OtherClientID"},
		{SubscriptionID: sub.ID, Status: "downloading", TorrentHash: "url:placeholder"},
		{SubscriptionID: sub.ID, Status: "organized", TorrentHash: "organized"},
		{SubscriptionID: other.ID, Status: "downloading", TorrentHash: "other"},
	}
	if err := database.DB.Create(&episodes).Error; err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("/api/subscriptions/%d/download-tasks", sub.ID)
	d := &presenceStub{found: map[string]bool{"abc": false, "def": true}}
	srv := &Server{downloader: downloader.NewDynamicDownloader(d)}
	read := func() map[string]string {
		t.Helper()
		w := webRequest(srv, http.MethodGet, url, "")
		if w.Code != 200 {
			t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
		}
		var v struct{ Tasks map[string]string }
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v.Tasks
	}
	got := read()
	slices.Sort(d.ids)
	if !slices.Equal(d.ids, []string{"abc", "def", "otherclientid"}) || len(got) != 3 || got["abc"] != "missing" || got["def"] != "present" || got["otherclientid"] != "unknown" {
		t.Fatalf("checked %v, got %+v", d.ids, got)
	}
	d.err = errors.New("temporarily disconnected")
	if w := webRequest(srv, http.MethodGet, url, ""); w.Code != 502 {
		t.Fatalf("offline: %d %s", w.Code, w.Body.String())
	}
	d.err = nil
	d.found["abc"] = true
	if read()["abc"] != "present" {
		t.Fatal("restored task stays missing")
	}
	var after []database.Episode
	if err := database.DB.Order("id").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	if len(after) != len(episodes) {
		t.Fatal("history length changed")
	}
	for i, row := range after {
		if row.Status != episodes[i].Status || row.TorrentHash != episodes[i].TorrentHash || row.ID != episodes[i].ID {
			t.Fatal("history changed")
		}
	}
	if w := webRequest(srv, http.MethodGet, "/api/subscriptions/999999/download-tasks", ""); w.Code != 404 {
		t.Fatalf("unknown subscription: %d", w.Code)
	}
	srv.downloader = nil
	if w := webRequest(srv, http.MethodGet, url, ""); w.Code != 503 {
		t.Fatalf("unconfigured: %d", w.Code)
	}
}
