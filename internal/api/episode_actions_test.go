package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

type episodeDeleteRecorder struct {
	stubDownloader
	hashes   []string
	failHash string
}

func (d *episodeDeleteRecorder) Delete(_ context.Context, hash string, files bool) error {
	if !files {
		return errors.New("expected file deletion")
	}
	if hash == d.failHash {
		return errors.New("downloader unavailable")
	}
	d.hashes = append(d.hashes, hash)
	return nil
}

func TestEpisodeDeletionScopedAndRetriable(t *testing.T) {
	setupTestDB(t)
	a := seedSubscription(t, database.Subscription{TitleCN: "甲"})
	b := seedSubscription(t, database.Subscription{TitleCN: "乙"})
	selected := database.Episode{SubscriptionID: a.ID, TorrentHash: "selected"}
	selected.ID = b.ID
	other := database.Episode{SubscriptionID: b.ID, TorrentHash: "other"}
	same := database.Episode{SubscriptionID: a.ID, TorrentHash: "same"}
	for _, ep := range []*database.Episode{&selected, &other, &same} {
		if err := database.DB.Create(ep).Error; err != nil {
			t.Fatal(err)
		}
	}
	dl := &episodeDeleteRecorder{}
	s := &Server{downloader: dl}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	send := func(ids string) int {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/subscriptions/%d/episodes/batch-delete", a.ID), strings.NewReader(fmt.Sprintf(`{"ids":[%s],"delete_files":true}`, ids)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}
	if code := send(fmt.Sprintf("%d,%d", selected.ID, other.ID)); code != 400 || len(dl.hashes) != 0 {
		t.Fatalf("mixed selection affected downloader: %d %v", code, dl.hashes)
	}
	dl.failHash = same.TorrentHash
	ids := fmt.Sprintf("%d,%d", selected.ID, same.ID)
	if code := send(ids); code != 502 {
		t.Fatalf("expected downloader failure, got %d", code)
	}
	dl.failHash = ""
	if code := send(ids); code != 200 {
		t.Fatalf("retry failed: %d", code)
	}
	if len(dl.hashes) != 2 || dl.hashes[0] != selected.TorrentHash || dl.hashes[1] != same.TorrentHash {
		t.Fatalf("wrong tasks %v", dl.hashes)
	}
	for _, sub := range []database.Subscription{a, b} {
		if err := database.DB.First(&database.Subscription{}, sub.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	var remaining []database.Episode
	if err := database.DB.Find(&remaining).Error; err != nil || len(remaining) != 1 || remaining[0].ID != other.ID {
		t.Fatalf("unexpected remaining episodes %+v (%v)", remaining, err)
	}
}

func TestManualEpisodeCompletionPreservesTransfer(t *testing.T) {
	setupTestDB(t)
	ep := database.Episode{SubscriptionID: 1, Status: "downloading", TorrentHash: "active"}
	if err := database.DB.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/episodes/1/status", strings.NewReader(`{"status":"completed"}`))
	req.SetPathValue("id", fmt.Sprint(ep.ID))
	w := httptest.NewRecorder()
	new(Server).handleUpdateEpisodeStatus(w, req)
	var stored database.Episode
	if err := database.DB.First(&stored, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	response := toEpisodeResponse(stored)
	if w.Code != 200 || response.Status != "downloading" || response.ManualStatus != "completed" {
		t.Fatalf("HTTP %d; saved episode %+v", w.Code, response)
	}
}

func TestPasswordRotationKeepsWizardSession(t *testing.T) {
	setupTestDB(t)
	if err := auth.InitSecret(); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDefaultUser(auth.HashPassword); err != nil {
		t.Fatal(err)
	}
	var user database.User
	if err := database.DB.First(&user).Error; err != nil {
		t.Fatal(err)
	}
	oldToken, err := auth.GenerateToken(user.Username, user.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	server := auth.AuthMiddleware(mux)
	send := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		return w
	}
	result := send("POST", "/api/user/change-password", `{"old_password":"admin","new_password":"test-new-password"}`, oldToken)
	var rotation struct{ Token string }
	if result.Code != 200 || json.Unmarshal(result.Body.Bytes(), &rotation) != nil || rotation.Token == "" {
		t.Fatalf("rotation failed: %d", result.Code)
	}
	payload := `{"settings":{"TV_BASE_PATH":"/test/media"}}`
	if w := send("PUT", "/api/settings", payload, oldToken); w.Code != 401 {
		t.Fatalf("old token accepted: %d", w.Code)
	}
	if w := send("PUT", "/api/settings", payload, rotation.Token); w.Code != 200 {
		t.Fatalf("new session rejected: %d", w.Code)
	}
	if actual := getSettingValue("TV_BASE_PATH"); actual != "/test/media" {
		t.Fatalf("setup not persisted: %q", actual)
	}
}
