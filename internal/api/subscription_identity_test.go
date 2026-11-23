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

func TestTMDBSubscriptionSeasonIdentity(t *testing.T) {
	setupTestDB(t)
	mux := http.NewServeMux()
	(&Server{}).registerRoutes(mux)
	const path = "/api/subscriptions"
	panelCall(t, mux, "POST", path, `{"title_cn":"第一季","metadata_id":"42","metadata_provider":"TMDB"}`, 201)
	// 旧订阅可能没有写入季号，同一作品的第一季仍应查重。
	if err := database.DB.Exec("UPDATE subscriptions SET season = NULL WHERE id = 1").Error; err != nil {
		t.Fatal(err)
	}
	panelCall(t, mux, "POST", path, `{"title_cn":"第一季别名","season":1,"metadata_id":"42","metadata_provider":"tmdb"}`, 409)
	panelCall(t, mux, "POST", path, `{"title_cn":"第二季","season":2,"metadata_id":"42","metadata_provider":"tmdb"}`, 201)
	panelCall(t, mux, "POST", path, `{"title_cn":"第二季别名","season":2,"metadata_id":"42","metadata_provider":"TMDB"}`, 409)
	panelCall(t, mux, "POST", path, `{"title_cn":"第三季","season":3}`, 201)
	panelCall(t, mux, "PUT", path+"/3", `{"metadata_id":"42","metadata_provider":"TmDb"}`, 200)
	panelCall(t, mux, "PUT", path+"/3", `{"season":2}`, 409)
	var saved database.Subscription
	if err := database.DB.First(&saved, 3).Error; err != nil || saved.Season != 3 || saved.MetadataID != "42" {
		t.Fatalf("冲突后第三季未保留原订阅：%+v，错误 %v", saved, err)
	}
	body := panelCall(t, mux, "POST", path+"/batch", `{"items":[{"title_cn":"第四季","season":4,"metadata_id":"42","metadata_provider":"TMDB"},{"title_cn":"同季别名","season":4,"metadata_id":"42","metadata_provider":"tmdb"}]}`, 201)
	var batch batchSubscriptionResponse
	if err := json.Unmarshal([]byte(body), &batch); err != nil || len(batch.Success) != 1 || len(batch.Failed) != 1 {
		t.Fatalf("批量创建结果错误：%s", body)
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

func TestTMDBIdentityUsesSeparateMovieAndTVNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name         string
		old          database.Subscription
		id           string
		season, want int
	}{
		{"旧裸号电视剧匹配带类型的同季", database.Subscription{MetadataID: "123", Season: 1}, "tv/123", 1, 409},
		{"电视剧保留不同季", database.Subscription{MetadataID: "tv/123", Season: 1}, "123", 2, 201},
		{"同号电影和电视剧可同名", database.Subscription{MetadataID: "123", TMDBID: "movie/123", TitleCN: "同名作品"}, "tv/123", 1, 201},
		{"同号裸电视剧与电影可并存", database.Subscription{MetadataID: "123", TMDBID: "movie/123"}, "123", 1, 201},
		{"电影不按季分割", database.Subscription{MetadataID: "123", TMDBID: "movie/123", Season: 2}, "movie/123", 1, 409},
		{"旧电影分类可用于同作品查重", database.Subscription{MetadataID: "123", AnimeType: "剧场版", Season: 3}, "movie/123", 1, 409},
		{"历史标识含空白", database.Subscription{MetadataID: "movie/123\n"}, "movie/123", 1, 409},
		{"仅有专用标识的旧记录", database.Subscription{TMDBID: "movie/123"}, "movie/123", 1, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTestDB(t)
			if tc.old.TitleCN == "" {
				tc.old.TitleCN = "旧作品"
			}
			tc.old.MetadataProvider = "TMDB"
			if err := database.DB.Create(&tc.old).Error; err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			(&Server{}).registerRoutes(mux)
			body, _ := json.Marshal(map[string]any{"title_cn": "同名作品", "metadata_provider": "TMDB", "metadata_id": tc.id, "season": tc.season})
			panelCall(t, mux, "POST", "/api/subscriptions", string(body), tc.want)
		})
	}
}

func TestTMDBIdentityChecksEditedMediaType(t *testing.T) {
	setupTestDB(t)
	mux := http.NewServeMux()
	(&Server{}).registerRoutes(mux)
	shows := []database.Subscription{{TitleCN: "电影", MetadataProvider: "TMDB", MetadataID: "123", AnimeType: "剧场版"}, {TitleCN: "电视剧", MetadataProvider: "TMDB", MetadataID: "123", AnimeType: "tv"}}
	if err := database.DB.Create(&shows).Error; err != nil {
		t.Fatal(err)
	}
	panelCall(t, mux, "PUT", "/api/subscriptions/2", `{"anime_type":"movie"}`, 409)
	var saved database.Subscription
	if err := database.DB.First(&saved, shows[1].ID).Error; err != nil || saved.AnimeType != "tv" {
		t.Fatalf("冲突编辑后类型为 %q，错误 %v", saved.AnimeType, err)
	}
}

func TestTMDBSubscriptionMetadataEditsReconcileStoredIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, provider, id, aux, body, canonical, lookup string
	}{
		{"更换已补全的作品", "TMDB", "tv/123", "tv/123", `{"metadata_id":"tv/456"}`, "tv/456", "tv/456"},
		{"重新提交错误记录的主标识", "TMDB", "tv/456", "tv/123", `{"metadata_id":"tv/456"}`, "tv/456", "tv/456"},
		{"清除主标识也清除旧回退值", "TMDB", "tv/123", "tv/123", `{"metadata_id":""}`, "", ""},
		{"未改标识保留历史电影类型", "TMDB", "123", "movie/123", `{"description":"新简介"}`, "movie/123", "movie/123"},
		{"重提交裸号保留电影类型", "TMDB", "123", "movie/123", `{"metadata_id":"123","metadata_provider":"tmdb"}`, "movie/123", "movie/123"},
		{"同一来源重提不删独立跨库映射", "BGM.tv", "5", "movie/123", `{"metadata_id":"5","metadata_provider":"bgm.TV"}`, "", ""},
		{"清空来源防止旧号变成历史回退值", "TMDB", "tv/123", "tv/123", `{"metadata_id":"","metadata_provider":""}`, "", ""},
		{"从其他来源转为 TMDB", "BGM.tv", "5", "movie/123", `{"metadata_id":"tv/456","metadata_provider":"TMDB"}`, "tv/456", "tv/456"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTestDB(t)
			sub := database.Subscription{TitleCN: "来源标识测试", Season: 1, MetadataProvider: tc.provider, MetadataID: tc.id, TMDBID: tc.aux}
			if err := database.DB.Create(&sub).Error; err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			(&Server{}).registerRoutes(mux)
			panelCall(t, mux, "PUT", "/api/subscriptions/1", tc.body, 200)
			var saved database.Subscription
			if err := database.DB.First(&saved, sub.ID).Error; err != nil {
				t.Fatal(err)
			}
			if strings.EqualFold(saved.MetadataProvider, "BGM.tv") && saved.TMDBID != tc.aux {
				t.Fatalf("BGM 的独立映射从 %q 变成了 %q", tc.aux, saved.TMDBID)
			}
			if canonical, lookup := saved.TMDBIdentity(); canonical != tc.canonical || lookup != tc.lookup {
				t.Fatalf("编辑后 metadata_id=%q tmdb_id=%q，查重标识和查询标识为 (%q, %q)，预期 (%q, %q)", saved.MetadataID, saved.TMDBID, canonical, lookup, tc.canonical, tc.lookup)
			}
		})
	}
}

func TestTMDBSubscriptionEditRejectsDuplicateAfterEnrichment(t *testing.T) {
	setupTestDB(t)
	mux := http.NewServeMux()
	(&Server{}).registerRoutes(mux)
	subs := []database.Subscription{
		{TitleCN: "待改作品", Season: 1, MetadataProvider: "TMDB", MetadataID: "tv/123", TMDBID: "tv/123"},
		{TitleCN: "另一部作品", Season: 1, MetadataProvider: "TMDB", MetadataID: "tv/456"},
	}
	if err := database.DB.Create(&subs).Error; err != nil {
		t.Fatal(err)
	}
	panelCall(t, mux, "PUT", "/api/subscriptions/1", `{"metadata_id":"tv/456","description":"冲突编辑"}`, 409)
	var saved database.Subscription
	if err := database.DB.First(&saved, subs[0].ID).Error; err != nil || saved.MetadataID != "tv/123" || saved.TMDBID != "tv/123" || saved.Description != "" {
		t.Fatalf("冲突未回滚：metadata_id=%q tmdb_id=%q description=%q 错误=%v", saved.MetadataID, saved.TMDBID, saved.Description, err)
	}
}
