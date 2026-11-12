package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&database.Subscription{}); err != nil {
		t.Fatalf("数据库迁移失败: %v", err)
	}
	return db
}

func TestCollectionMerger_LocalFallback(t *testing.T) {
	db := setupTestDB(t)
	merger := NewCollectionMerger(db, nil)
	ctx := context.Background()

	sub1 := database.Subscription{
		TitleCN:  "鬼灭之刃 锻刀村篇",
		SeriesID: "",
	}
	db.Create(&sub1)

	sub2 := database.Subscription{
		TitleCN:  "鬼灭之刃 游郭篇",
		SeriesID: "鬼灭之刃旧系列",
	}
	db.Create(&sub2)

	// 1. 测试单条解析
	info, err := merger.ResolveSingleSubscription(ctx, &sub1)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if info.SeriesID != "鬼灭之刃" {
		t.Errorf("期望系列名 '鬼灭之刃', 实际得到: %s", info.SeriesID)
	}

	// 2. 生成不覆盖计划 (overwrite = false)
	subs := []database.Subscription{sub1, sub2}
	summary, err := merger.GenerateMergePlans(ctx, subs, false)
	if err != nil {
		t.Fatalf("生成归并计划失败: %v", err)
	}

	if len(summary.Plans) != 2 {
		t.Fatalf("计划数量期望 2, 实际得到: %d", len(summary.Plans))
	}
	if !summary.Plans[0].ShouldApply {
		t.Errorf("未设置系列的 sub1 应标记为应用归并")
	}
	if summary.Plans[1].ShouldApply {
		t.Errorf("已有系列的 sub2 在 overwrite=false 时不应被覆盖")
	}

	// 3. 执行归并落库
	applied, err := merger.ApplyMergePlans(ctx, summary.Plans)
	if err != nil {
		t.Fatalf("应用归并失败: %v", err)
	}
	if applied != 1 {
		t.Errorf("应用条数期望 1, 实际得到: %d", applied)
	}

	var updatedSub1 database.Subscription
	db.First(&updatedSub1, sub1.ID)
	if updatedSub1.SeriesID != "鬼灭之刃" {
		t.Errorf("落库系列名不匹配: %s", updatedSub1.SeriesID)
	}

	// 4. 生成强制覆盖计划 (overwrite = true)
	summaryOverwrite, err := merger.GenerateMergePlans(ctx, subs, true)
	if err != nil {
		t.Fatalf("生成强制归并计划失败: %v", err)
	}
	if !summaryOverwrite.Plans[1].ShouldApply {
		t.Errorf("已有系列的 sub2 在 overwrite=true 时应被标记为覆盖")
	}
}

func TestCollectionMerger_TMDBClusterAutoMerge(t *testing.T) {
	// 模拟 TMDB API，提供 TV 详情、电影详情、电影合集以及合集查询
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/movie/635302"):
			// 无限列车篇电影
			w.Write([]byte(`{
				"id": 635302,
				"title": "鬼灭之刃 无限列车篇",
				"belongs_to_collection": {
					"id": 726417,
					"name": "鬼灭之刃（系列）"
				}
			}`))
		case strings.HasPrefix(r.URL.Path, "/collection/726417"):
			w.Write([]byte(`{
				"id": 726417,
				"name": "鬼灭之刃（系列）",
				"parts": [
					{"id": 635302, "media_type": "movie"},
					{"id": 85937, "media_type": "tv"}
				]
			}`))
		case strings.HasPrefix(r.URL.Path, "/tv/85937"):
			w.Write([]byte(`{
				"id": 85937,
				"name": "鬼灭之刃",
				"original_name": "鬼滅の刃"
			}`))
		case strings.HasPrefix(r.URL.Path, "/search/collection"):
			w.Write([]byte(`{
				"page": 1,
				"results": [{"id": 726417, "name": "鬼灭之刃（系列）"}]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	db := setupTestDB(t)
	tmdb := NewTMDBProvider("test_key", "zh-CN", []string{server.URL})
	merger := NewCollectionMerger(db, tmdb)

	ctx := context.Background()

	// 插入三部属于同一系列的不同类型作品：TV 季 1、电影、TV 季 2
	subTV1 := database.Subscription{
		TitleCN:  "鬼灭之刃 灶门炭治郎 立志篇",
		TMDBID:   "tv/85937",
		SeriesID: "",
	}
	subMovie := database.Subscription{
		TitleCN:  "剧场版 鬼灭之刃 无限列车篇",
		TMDBID:   "movie/635302",
		SeriesID: "",
	}
	subTV2 := database.Subscription{
		TitleCN:  "鬼灭之刃 游郭篇",
		TMDBID:   "85937",
		SeriesID: "",
	}

	db.Create(&subTV1)
	db.Create(&subMovie)
	db.Create(&subTV2)

	// 通过 AutoMergeSubscriptions 从 DB 批量读取并自动归并
	summary, err := merger.AutoMergeSubscriptions(ctx, false)
	if err != nil {
		t.Fatalf("AutoMergeSubscriptions 失败: %v", err)
	}

	if summary.TotalProcessed != 3 {
		t.Errorf("处理数量期望 3, 实际 %d", summary.TotalProcessed)
	}
	if summary.TotalApplied != 3 {
		t.Errorf("应用条数期望 3, 实际 %d", summary.TotalApplied)
	}

	var results []database.Subscription
	db.Find(&results)
	for _, res := range results {
		if res.SeriesID != "鬼灭之刃" {
			t.Errorf("番剧 [%s] 归并后的 SeriesID 期望 '鬼灭之刃', 实际 '%s'", res.TitleCN, res.SeriesID)
		}
	}
}
