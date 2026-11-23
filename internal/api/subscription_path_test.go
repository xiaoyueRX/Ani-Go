package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func TestSubscriptionDownloadDirectoryCanBeCleared(t *testing.T) {
	setupTestDB(t)
	sub := database.Subscription{TitleCN: "Series", CustomPath: "/client/incoming"}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Server{}).registerRoutes(mux)
	for _, tc := range []struct {
		body, want string
		status     int
	}{
		{`{"year":2026}`, "/client/incoming", 200}, {`{"custom_path":null}`, "", 200},
		{`{"custom_path":"/other/path"}`, "/other/path", 200}, {`{"custom_path":123}`, "/other/path", 400}, {`{"custom_path":""}`, "", 200},
	} {
		panelCall(t, mux, "PUT", fmt.Sprintf("/api/subscriptions/%d", sub.ID), tc.body, tc.status)
		var saved database.Subscription
		if err := database.DB.First(&saved, sub.ID).Error; err != nil {
			t.Fatal(err)
		}
		if saved.CustomPath != tc.want {
			t.Fatalf("after %s directory=%q want %q", tc.body, saved.CustomPath, tc.want)
		}
	}
}
