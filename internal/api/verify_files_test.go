package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

func TestVerifyFilesReportsEachEditionWithoutChangingRecords(t *testing.T) {
	setupTestDB(t)
	t.Setenv("TV_VOLUME_ID", "")
	base := t.TempDir()
	s := &Server{organizer: organizer.New("", "", "", base, t.TempDir(), true, nil)}
	sub := seedSubscription(t, database.Subscription{TitleCN: "Archive", AnimeType: "tv", TotalEpisodes: 2})
	other := seedSubscription(t, database.Subscription{TitleCN: "Other", AnimeType: "movie"})
	write := func(name, content string) string {
		p := filepath.Join(base, name)
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	present := write("feature.mkv", "test")
	changed := write("edition.mkv", "other")
	missing := filepath.Join(base, "missing.mkv")
	outside := filepath.Join(t.TempDir(), "private.mkv")
	route := fmt.Sprintf("/api/subscriptions/%d/verify-files", sub.ID)
	ep := func(path, status string, size int64) database.Episode {
		return database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, Status: status, FinalPath: path, FileSize: size, SourceFile: "source.mkv", SourceModTime: 12}
	}
	rows := []database.Episode{ep(present, "organized", 4), ep(changed, "organized", 4), ep(missing, "organized", 4), ep(outside, "organized", 4), ep("", "organized", 4), ep(present, "downloading", 4), ep(present, "organized", 0), ep(filepath.Join(base, "document.txt"), "organized", 4)}
	rows[4].ManualStatus = "failed"
	rows[5].ManualStatus = "completed"
	for i := range rows {
		if err := database.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	foreign := ep(outside, "organized", 4)
	foreign.SubscriptionID = other.ID
	if err := database.DB.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	deleted := ep(outside, "organized", 4)
	if err := database.DB.Create(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Delete(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	var before []database.Episode
	if err := database.DB.Order("id").Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	type result struct {
		ID             uint
		Status, Reason string
		Expected       *int64 `json:"expected_size"`
		Actual         *int64 `json:"actual_size"`
	}
	request := func(ids []uint, guard bool) []result {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"ids": ids})
		w := webRequest(s, "POST", route, string(body))
		if w.Code != 200 {
			t.Fatalf("核对失败：HTTP %d %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), base) || strings.Contains(w.Body.String(), outside) || strings.Contains(w.Body.String(), "feature.mkv") {
			t.Fatalf("核对暴露本机路径或文件名：%s", w.Body.String())
		}
		var got struct {
			Results []result
			Guarded bool `json:"volume_guarded"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Results) != len(ids) || got.Guarded != guard {
			t.Fatalf("核对结果错误：%+v", got)
		}
		for i := range ids {
			if got.Results[i].ID != ids[i] {
				t.Fatal("返回次序错误")
			}
		}
		return got.Results
	}
	ids := []uint{rows[1].ID, rows[0].ID, rows[2].ID, rows[3].ID, rows[4].ID, rows[5].ID, rows[6].ID, rows[7].ID}
	want := []string{"size_mismatch", "size_matches", "missing", "unverified", "unverified", "not_applicable", "present", "unverified"}
	for twice := 0; twice < 2; twice++ {
		got := request(ids, false)
		for i := range want {
			if got[i].Status != want[i] {
				t.Errorf("%d: %+v, 期望 %s", ids[i], got[i], want[i])
			}
		}
		if got[0].Expected == nil || got[0].Actual == nil || *got[0].Expected != 4 || *got[0].Actual != 5 {
			t.Fatalf("大小证据错误：%+v", got[0])
		}
	}
	t.Setenv("TV_VOLUME_ID", "real-volume")
	write(".anigo-volume-id", "wrong-volume")
	got := request([]uint{rows[2].ID, rows[0].ID}, true)
	for _, report := range got {
		if report.Status != "unverified" || report.Reason != "volume_unavailable" {
			t.Fatalf("错卷应当不可核对：%+v", report)
		}
	}
	write(".anigo-volume-id", "real-volume\n")
	got = request([]uint{rows[2].ID, rows[0].ID}, true)
	if got[0].Status != "missing" || got[1].Status != "size_matches" {
		t.Fatalf("正确卷结果错误：%+v", got)
	}
	for _, badID := range []uint{foreign.ID, deleted.ID, deleted.ID + 100} {
		if w := webRequest(s, "POST", route, fmt.Sprintf(`{"ids":[%d,%d]}`, rows[0].ID, badID)); w.Code != 404 || strings.Contains(w.Body.String(), "results") {
			t.Fatalf("混合归属须整批拒绝：%d %s", w.Code, w.Body.String())
		}
	}
	for _, body := range []string{`{}`, `{"ids":[]}`, `{"ids":[0]}`, `{"ids":[1],"path":"/"}`, `{"ids":[1]}{}`, `{"ids":[` + strings.Repeat("1,", 100) + "1]}"} {
		if w := webRequest(s, "POST", route, body); w.Code != 400 {
			t.Fatalf("无效选择应为400，得到 %d", w.Code)
		}
	}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	w := httptest.NewRecorder()
	auth.AuthMiddleware(mux).ServeHTTP(w, httptest.NewRequest("POST", route, strings.NewReader(`{"ids":[1]}`)))
	if w.Code != 401 {
		t.Fatalf("未登录应为401，得到 %d", w.Code)
	}
	var after []database.Episode
	if err := database.DB.Order("id").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("核对更改了剧集记录")
	}
	if data, err := os.ReadFile(changed); err != nil || string(data) != "other" {
		t.Fatal("核对更改了文件")
	}
}
