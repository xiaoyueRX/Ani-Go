package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func TestCatalogSeasonInSubscriptionCreationAndPreview(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()
	for _, tc := range []struct {
		title  string
		season *int
		want   int
	}{{"星岛 第二季", nil, 2}, {"星岛 Another Season 3", nil, 3}, {"手选作品 第二季", new(int), 1},
		{"第一季＋第二季", nil, 1}, {"合季 Season 2-3", nil, 1}, {"作品 2026", nil, 1}} {
		t.Run(tc.title, func(t *testing.T) {
			input := map[string]any{"title_cn": tc.title, "mikan_id": "catalog-" + tc.title}
			if tc.season != nil {
				input["season"] = *tc.season
			}
			body, _ := json.Marshal(input)
			req := httptest.NewRequest(http.MethodPost, "/api/subscriptions", bytes.NewReader(body))
			w := httptest.NewRecorder()
			s.handleCreateSubscription(w, req)
			var sub subscriptionResponse
			if err := json.Unmarshal(w.Body.Bytes(), &sub); err != nil || w.Code != 201 || sub.Season != tc.want || sub.TitleCN != tc.title {
				t.Fatalf("创建后季号=%d，作品名=%q，期望%d：%s", sub.Season, sub.TitleCN, tc.want, w.Body.String())
			}
		})
	}
	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions/batch", bytes.NewBufferString(`{"items":[{"title_cn":"批量 2nd Season","mikan_id":"22"},{"title_cn":"手选 第二季","mikan_id":"23","season":1}]}`))
	w := httptest.NewRecorder()
	s.handleBatchCreateSubscriptions(w, req)
	for id, wanted := range map[string]int{"22": 2, "23": 1} {
		var sub database.Subscription
		if err := database.DB.Where("mikan_id = ?", id).First(&sub).Error; err != nil || sub.Season != wanted {
			t.Fatalf("批量 %s 季号 %d，期望%d：%v", id, sub.Season, wanted, err)
		}
	}
	req = httptest.NewRequest(http.MethodPost, "/api/settings/custom-regex/test", bytes.NewBufferString(`{"title":"星岛 第二季"}`))
	w = httptest.NewRecorder()
	s.handleTestCustomRegex(w, req)
	var preview struct {
		CatalogSeason int `json:"catalog_season"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil || preview.CatalogSeason != 2 {
		t.Fatalf("审阅未给出同一季号：%s", w.Body.String())
	}
}
