package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func TestSubscriptionEditRefreshesSeasonProgress(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()
	sub := seedSubscription(t, database.Subscription{TitleCN: "分季进度", Season: 1, CurrentEpisodes: 1})
	if err := database.DB.Create(&database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, Status: "organized"}).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/subscriptions/%d", sub.ID)
	for _, tc := range []struct {
		body      string
		current   int
		completed bool
	}{
		{`{"total_episodes":1}`, 1, true},
		{`{"total_episodes":2}`, 1, false},
		{`{"total_episodes":1,"completed":false}`, 1, false},
		{`{"description":"修改简介"}`, 1, false},
		{`{"total_episodes":1,"season":1}`, 1, false},
		{`{"season":2,"completed":true}`, 0, true},
		{`{"season":1}`, 1, true},
		{`{"season":2}`, 0, false},
		{`{"completed":true}`, 0, true},
		{`{"total_episodes":0}`, 0, true},
	} {
		w := webRequest(s, http.MethodPut, path, tc.body)
		if w.Code != http.StatusOK {
			t.Fatalf("更新 %s 失败: %s", tc.body, w.Body.String())
		}
		var got subscriptionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.CurrentEpisodes != tc.current || got.Completed != tc.completed {
			t.Errorf("更新 %s 后返回 %d 集、完结 %t，期望 %d 集、完结 %t", tc.body, got.CurrentEpisodes, got.Completed, tc.current, tc.completed)
		}
		var saved database.Subscription
		if err := database.DB.First(&saved, sub.ID).Error; err != nil {
			t.Fatal(err)
		}
		if saved.CurrentEpisodes != tc.current || saved.Completed != tc.completed {
			t.Errorf("更新 %s 后数据库为 %d 集、完结 %t", tc.body, saved.CurrentEpisodes, saved.Completed)
		}
	}
}

func TestSubscriptionEditRollsBackWhenProgressRefreshFails(t *testing.T) {
	setupTestDB(t)
	sub := seedSubscription(t, database.Subscription{TitleCN: "回滚", Season: 1, TotalEpisodes: 2})
	if err := database.DB.Exec("CREATE TRIGGER reject_progress BEFORE UPDATE OF current_episodes ON subscriptions BEGIN SELECT RAISE(ABORT,'progress unavailable'); END").Error; err != nil {
		t.Fatal(err)
	}
	w := webRequest(newTestServer(), http.MethodPut, fmt.Sprintf("/api/subscriptions/%d", sub.ID), `{"total_episodes":1}`)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("进度更新失败时 HTTP %d", w.Code)
	}
	var saved database.Subscription
	if err := database.DB.First(&saved, sub.ID).Error; err != nil || saved.TotalEpisodes != 2 {
		t.Fatalf("原总集数未保留: %d, %v", saved.TotalEpisodes, err)
	}
}
