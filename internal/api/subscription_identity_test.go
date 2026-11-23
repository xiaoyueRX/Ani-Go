package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func TestSubscriptionCreationDistinguishesSeasonsAndPreservesSourceIDs(t *testing.T) {
	setupTestDB(t)
	mux := http.NewServeMux()
	(&Server{}).registerRoutes(mux)
	const route = "/api/subscriptions"
	create := func(body string, expected int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != expected {
			t.Fatalf("创建 %s 返回 %d（预期 %d）: %s", body, w.Code, expected, w.Body.String())
		}
	}
	create(`{"title_cn":"Harbor Story"}`, 201)
	create(`{"title_cn":"Harbor Story","season":2}`, 201)
	create(`{"title_cn":"Harbor Story","season":2}`, 409)
	for _, old := range []string{"0", "NULL"} {
		if err := database.DB.Exec("UPDATE subscriptions SET season = "+old+" WHERE title_cn = ? AND id = 1", "Harbor Story").Error; err != nil {
			t.Fatal(err)
		}
		create(`{"title_cn":"Harbor Story","season":1}`, 409)
	}
	create(`{"title_cn":"码头","season":2,"mikan_id":"100","bangumi_id":"100"}`, 201)
	create(`{"title_cn":"码头别名","season":3,"mikan_id":"100"}`, 409)
	create(`{"title_cn":"归航","season":1,"metadata_id":"100","metadata_provider":"BGM.tv"}`, 201)
	create(`{"title_cn":"归航别名","season":2,"metadata_id":"100","metadata_provider":"bgm.TV"}`, 409)
	create(`{"title_cn":"旧来源","bangumi_id":"100"}`, 409)
	create(`{"title_cn":"Harbor Story","season":-1}`, 400)
	req := httptest.NewRequest(http.MethodPost, route+"/batch", strings.NewReader(`{"items":[{"title_cn":"Harbor Story","season":3},{"title_cn":"Harbor Story","season":3},{"title_cn":"Invalid","season":-1}]}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var batch batchSubscriptionResponse
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &batch) != nil || len(batch.Success) != 1 || len(batch.Failed) != 2 {
		t.Fatalf("批量结果为 %d %s", w.Code, w.Body.String())
	}
	var saved database.Subscription
	if err := database.DB.First(&saved, batch.Success[0].ID).Error; err != nil || saved.Season != 3 {
		t.Fatalf("批量生成季 %d 错误 %v", saved.Season, err)
	}
	if err := database.DB.Delete(&saved).Error; err != nil {
		t.Fatal(err)
	}
	create(`{"title_cn":"Harbor Story","season":3}`, 201)
}

func TestConcurrentSearchAndBatchSubscriptionCreation(t *testing.T) {
	setupTestDB(t)
	mux := http.NewServeMux()
	(&Server{}).registerRoutes(mux)
	const body = `{"title_cn":"并行订阅","season":2}`
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			path, payload := "/api/subscriptions", body
			if i%2 == 0 {
				path += "/batch"
				payload = `{"items":[` + body + `]}`
			}
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != 201 && w.Code != 409 {
				t.Errorf("创建返回 %d", w.Code)
			}
		}()
	}
	close(start)
	wg.Wait()
	var n int64
	if err := database.DB.Model(&database.Subscription{}).Where("title_cn = ? AND season = 2", "并行订阅").Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("并发创建生成 %d 项，错误 %v", n, err)
	}
}

func TestEditedSubscriptionPreservesIdentityAndAllowsEditingLegacyDuplicates(t *testing.T) {
	setupTestDB(t)
	mux := http.NewServeMux()
	(&Server{}).registerRoutes(mux)
	const path = "/api/subscriptions"
	panelCall(t, mux, "POST", path, `{"title_cn":"Two Seasons","season":1}`, 201)
	panelCall(t, mux, "POST", path, `{"title_cn":"Two Seasons","season":2}`, 201)
	panelCall(t, mux, "PUT", path+"/2", `{"season":1}`, 409)
	var existing database.Subscription
	if err := database.DB.First(&existing, 2).Error; err != nil || existing.Season != 2 {
		t.Fatalf("冲突没有保留原季 %d, %v", existing.Season, err)
	}
	panelCall(t, mux, "PUT", path+"/2", `{"season":3}`, 200)
	if err := database.DB.Model(&existing).Update("season", 1).Error; err != nil {
		t.Fatal(err)
	}
	panelCall(t, mux, "PUT", path+"/2", `{"enabled":false,"title_cn":"Two Seasons","season":1}`, 200)
	panelCall(t, mux, "PUT", path+"/2", `{"description":"更新历史重复记录"}`, 200)
	panelCall(t, mux, "PUT", path+"/2", `{"season":-3}`, 400)
}
