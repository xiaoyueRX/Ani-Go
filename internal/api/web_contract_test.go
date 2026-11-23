package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

type episodeDeleteRecorder struct {
	stubDownloader
	deleted []string
}

func (d *episodeDeleteRecorder) Delete(_ context.Context, hash string, files bool) error {
	if !files {
		return fmt.Errorf("未请求删除下载文件")
	}
	d.deleted = append(d.deleted, hash)
	return nil
}

func webRequest(s *Server, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func TestEpisodeDeletionStaysWithinSubscription(t *testing.T) {
	setupTestDB(t)
	first := seedSubscription(t, database.Subscription{TitleCN: "第一部"})
	other := seedSubscription(t, database.Subscription{TitleCN: "第二部"})
	episodes := []database.Episode{
		{SubscriptionID: other.ID, Number: 1, TorrentHash: "保留"},
		{SubscriptionID: first.ID, Number: 1, TorrentHash: "删除"},
	}
	if err := database.DB.Create(&episodes).Error; err != nil {
		t.Fatal(err)
	}
	s := newTestServer()
	dl := &episodeDeleteRecorder{}
	s.downloader = dl
	path := fmt.Sprintf("/api/subscriptions/%d/episodes/batch-delete", first.ID)
	if w := webRequest(s, "POST", path, fmt.Sprintf(`{"ids":[%d,%d],"delete_files":true}`, episodes[0].ID, episodes[1].ID)); w.Code != 404 || len(dl.deleted) != 0 {
		t.Fatalf("跨订阅请求应拒绝且无副作用，HTTP %d，删除 %v", w.Code, dl.deleted)
	}
	if w := webRequest(s, "POST", path, fmt.Sprintf(`{"ids":[%d],"delete_files":true}`, episodes[1].ID)); w.Code != 200 {
		t.Fatalf("删除单集应成功，HTTP %d: %s", w.Code, w.Body.String())
	}
	if len(dl.deleted) != 1 || dl.deleted[0] != "删除" {
		t.Fatal(dl.deleted)
	}
	var n int64
	database.DB.Model(&database.Subscription{}).Count(&n)
	if n != 2 {
		t.Fatal("其他订阅受影响")
	}
	database.DB.Model(&database.Episode{}).Count(&n)
	if n != 1 {
		t.Fatal("仅应删除选中的一集")
	}
}

func TestCreateSubscriptionPreservesSeasonAndGroups(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()
	w := webRequest(s, "POST", "/api/subscriptions", `{"title_cn":"第二季","season":2,"allowed_subgroups":["ANi"]}`)
	var sub subscriptionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &sub); err != nil || w.Code != 201 || sub.Season != 2 || sub.AllowedSubgroups != `["ANi"]` {
		t.Fatalf("季号或字幕组错误，HTTP %d %s", w.Code, w.Body.String())
	}
	w = webRequest(s, "POST", "/api/subscriptions/batch", `{"items":[{"title_cn":"多组","subgroups":["ANi","LoliHouse"]}]}`)
	var stored database.Subscription
	if err := database.DB.Where("title_cn = ?", "多组").First(&stored).Error; err != nil || w.Code != 201 || stored.SubgroupName != "" || stored.AllowedSubgroups != `["ANi","LoliHouse"]` {
		t.Fatalf("多组筛选记录错误，%+v", stored)
	}
}

func TestManualEpisodeStatusLeavesDownloadAndProgressIntact(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()
	sub := seedSubscription(t, database.Subscription{TitleCN: "测试番剧", Season: 1, TotalEpisodes: 2, CurrentEpisodes: 1})
	episodes := []database.Episode{{SubscriptionID: sub.ID, Season: 1, Number: 1, Status: "organized"}, {SubscriptionID: sub.ID, Season: 1, Number: 2, TorrentHash: "b", Status: "downloading"}}
	if err := database.DB.Create(&episodes).Error; err != nil {
		t.Fatal(err)
	}
	ep := episodes[1]
	url := fmt.Sprintf("/api/episodes/%d/status", ep.ID)
	for _, mark := range []string{"completed", "completed", "failed", "pending", "downloading", ""} {
		w := webRequest(s, "PUT", url, fmt.Sprintf(`{"status":%q}`, mark))
		var ack map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &ack); err != nil || w.Code != 200 || len(ack) != 2 || ack["status"] != mark || ack["manual_status"] != mark {
			t.Fatalf("标记 %q 回执错误，HTTP %d %s", mark, w.Code, w.Body.String())
		}
		w = webRequest(s, "GET", fmt.Sprintf("/api/subscriptions/%d", sub.ID), "")
		var detail struct {
			Subscription subscriptionResponse `json:"subscription"`
			Episodes     []map[string]any     `json:"episodes"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil || w.Code != 200 || detail.Subscription.CurrentEpisodes != 1 || detail.Subscription.Completed {
			t.Fatalf("标记 %q 改写了进度，%s", mark, w.Body.String())
		}
		found := false
		for _, item := range detail.Episodes {
			if item["id"] == float64(ep.ID) {
				label, marked := item["manual_status"]
				found = item["status"] == "downloading" && ((mark == "" && !marked) || label == mark)
			}
		}
		if !found {
			t.Fatalf("标记 %q 未独立展示：%s", mark, w.Body.String())
		}
		var persisted database.Episode
		if err := database.DB.First(&persisted, ep.ID).Error; err != nil || !persisted.UpdatedAt.Equal(ep.UpdatedAt) {
			t.Fatalf("手工标记影响调度重试时钟：%+v %v", persisted, err)
		}
	}
	for _, body := range []string{`{}`, `{"status":null}`, `{"status":"organized"}`} {
		if w := webRequest(s, "PUT", url, body); w.Code != 400 {
			t.Fatalf("应拒绝 %s：HTTP %d", body, w.Code)
		}
	}
	if w := webRequest(s, "PUT", "/api/episodes/99999/status", `{"status":""}`); w.Code != 404 {
		t.Fatalf("不存在的剧集应返回 404：%d", w.Code)
	}
}

func TestSearchCapabilitiesMatchActiveExpander(t *testing.T) {
	for _, tc := range []struct {
		s    *Server
		want bool
	}{{newSmartServer(t, stubAI{}), true}, {newTestServer(), false}} {
		w := webRequest(tc.s, "GET", "/api/search/capabilities", "")
		var got struct {
			Smart bool `json:"smart_search"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 || got.Smart != tc.want {
			t.Fatalf("智能搜索能力错误，期望 %v，HTTP %d %s", tc.want, w.Code, w.Body.String())
		}
	}
}

func TestCreateSubscriptionKeepsDistinctSourceIDs(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()
	w := webRequest(s, "POST", "/api/subscriptions", `{"title_cn":"蜜柑番剧","bangumi_id":"12","mikan_id":"12"}`)
	var sub map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &sub); err != nil || w.Code != 201 || sub["mikan_id"] != "12" {
		t.Fatalf("蜜柑 ID 未保留，%s", w.Body.String())
	}
	w = webRequest(s, "POST", "/api/subscriptions/batch", `{"items":[{"title_cn":"班固米番剧","bangumi_id":"12","metadata_id":"12","metadata_provider":"BGM.tv"}]}`)
	var batch batchSubscriptionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &batch); err != nil || len(batch.Success) != 1 {
		t.Fatalf("不同站点编号碰撞，%s", w.Body.String())
	}
	w = webRequest(s, "POST", "/api/subscriptions", `{"title_cn":"班固米别名","bangumi_id":"12","metadata_id":"12","metadata_provider":"BGM.tv"}`)
	if w.Code != 409 {
		t.Fatalf("同来源同作品重复创建，%s", w.Body.String())
	}
}

func TestEpisodeDeletionProtectsSharedTorrent(t *testing.T) {
	setupTestDB(t)
	sub := seedSubscription(t, database.Subscription{TitleCN: "合集"})
	episodes := []database.Episode{{SubscriptionID: sub.ID, TorrentHash: "同一个合集", Number: 1}, {SubscriptionID: sub.ID, TorrentHash: "同一个合集", Number: 2}}
	if err := database.DB.Create(&episodes).Error; err != nil {
		t.Fatal(err)
	}
	s := newTestServer()
	dl := &episodeDeleteRecorder{}
	s.downloader = dl
	path := fmt.Sprintf("/api/subscriptions/%d/episodes/batch-delete", sub.ID)
	w := webRequest(s, "POST", path, fmt.Sprintf(`{"ids":[%d],"delete_files":true}`, episodes[0].ID))
	if w.Code != 409 || len(dl.deleted) != 0 {
		t.Fatalf("同一合集仍被其他剧集使用，HTTP %d 删除 %v", w.Code, dl.deleted)
	}
	w = webRequest(s, "POST", path, fmt.Sprintf(`{"ids":[%d],"delete_files":false}`, episodes[0].ID))
	if w.Code != 200 || len(dl.deleted) != 0 {
		t.Fatalf("应仅删记录，HTTP %d 删除 %v", w.Code, dl.deleted)
	}
}

func TestFileDeletionChecksAllOwnersIncludingRecycleBin(t *testing.T) {
	for _, route := range []string{"episode", "single", "batch"} {
		for _, recycled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/recycled=%v", route, recycled), func(t *testing.T) {
				setupTestDB(t)
				s := newTestServer()
				dl := &episodeDeleteRecorder{}
				s.downloader = dl
				first := seedSubscription(t, database.Subscription{TitleCN: "删"})
				keep := seedSubscription(t, database.Subscription{TitleCN: "留"})
				hash := strings.Repeat("aB", 20)
				items := []database.Episode{{SubscriptionID: first.ID, TorrentHash: hash, Number: 1}, {SubscriptionID: keep.ID, TorrentHash: strings.ToLower(hash), Number: 2}}
				if err := database.DB.Create(&items).Error; err != nil {
					t.Fatal(err)
				}
				if recycled {
					w := webRequest(s, "DELETE", fmt.Sprintf("/api/subscriptions/%d", keep.ID), "")
					if w.Code != 200 {
						t.Fatal(w.Body.String())
					}
				}
				var w *httptest.ResponseRecorder
				switch route {
				case "episode":
					w = webRequest(s, "POST", fmt.Sprintf("/api/subscriptions/%d/episodes/batch-delete", first.ID), fmt.Sprintf(`{"ids":[%d],"delete_files":true}`, items[0].ID))
				case "single":
					w = webRequest(s, "DELETE", fmt.Sprintf("/api/subscriptions/%d?delete_files=true", first.ID), "")
				case "batch":
					w = webRequest(s, "POST", "/api/subscriptions/batch-delete", fmt.Sprintf(`{"ids":[%d],"delete_files":true}`, first.ID))
				}
				if w.Code != 409 || len(dl.deleted) != 0 {
					t.Fatalf("共享文件应保留，HTTP %d，已删 %v", w.Code, dl.deleted)
				}
				var alive int64
				database.DB.Model(&database.Episode{}).Where("id = ?", items[0].ID).Count(&alive)
				if alive != 1 {
					t.Fatal("被拒的删除应保留所选剧集")
				}
			})
		}
	}
}

func TestDeletingAllTorrentOwnersCallsDownloaderOnce(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()
	dl := &episodeDeleteRecorder{}
	s.downloader = dl
	first := seedSubscription(t, database.Subscription{TitleCN: "甲"})
	second := seedSubscription(t, database.Subscription{TitleCN: "乙"})
	hash := strings.Repeat("aB", 20)
	items := []database.Episode{{SubscriptionID: first.ID, TorrentHash: hash}, {SubscriptionID: second.ID, TorrentHash: strings.ToUpper(hash)}}
	if err := database.DB.Create(&items).Error; err != nil {
		t.Fatal(err)
	}
	w := webRequest(s, "POST", "/api/subscriptions/batch-delete", fmt.Sprintf(`{"ids":[%d,%d],"delete_files":true}`, first.ID, second.ID))
	if w.Code != 200 || len(dl.deleted) != 1 || !strings.EqualFold(dl.deleted[0], hash) {
		t.Fatalf("同一任务只能删除一次，HTTP %d 已删 %v", w.Code, dl.deleted)
	}
}
