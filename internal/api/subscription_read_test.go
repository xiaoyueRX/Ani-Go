package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

func TestSubscriptionDetailReportsDatabaseReadFailures(t *testing.T) {
	for _, table := range []string{"subscriptions", "episodes"} {
		t.Run(table, func(t *testing.T) {
			setupTestDB(t)
			if err := database.DB.Create(&database.Subscription{TitleCN: "示例"}).Error; err != nil {
				t.Fatal(err)
			}
			callback := database.DB.Callback().Query()
			err := callback.Before("gorm:query").Register("test:detail-read", func(tx *gorm.DB) {
				if tx.Statement.Table == table && len(tx.Statement.Selects) == 0 {
					tx.AddError(errors.New("fixture read failure"))
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			defer callback.Remove("test:detail-read")
			mux := http.NewServeMux()
			(&Server{}).registerRoutes(mux)
			panelCall(t, mux, "GET", "/api/subscriptions/1", "", http.StatusInternalServerError)
		})
	}
}

func TestBatchStalledCountsUsesPartialIndex(t *testing.T) {
	previous := database.DB
	if err := database.Init(filepath.Join(t.TempDir(), "stalled-plan.sqlite")); err != nil {
		t.Fatal(err)
	}
	db := database.DB
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close(); database.DB = previous })

	if err := db.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<1000)
		INSERT INTO episodes(subscription_id,number,status,created_at) SELECT 1,i,'organized',datetime('now') FROM n`).Error; err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := db.Create(&database.Episode{
		SubscriptionID:    1,
		Season:            1,
		Number:            1001,
		Status:            "downloading",
		DownloadStartedAt: &old,
	}).Error; err != nil {
		t.Fatal(err)
	}

	type query struct {
		statement string
		args      []any
	}
	var queries []query
	callback := db.Callback().Query()
	if err := callback.After("gorm:query").Register("test:stalled_queries", func(tx *gorm.DB) {
		queries = append(queries, query{tx.Statement.SQL.String(), append([]any(nil), tx.Statement.Vars...)})
	}); err != nil {
		t.Fatal(err)
	}

	empty := batchStalledCounts(nil, 24*time.Hour)
	if len(empty) != 0 || len(queries) != 0 {
		_ = callback.Remove("test:stalled_queries")
		t.Fatalf("空订阅列表不应查询数据库，实际 map=%v queries=%d", empty, len(queries))
	}

	counts := batchStalledCounts([]uint{1, 2}, 24*time.Hour)
	_ = callback.Remove("test:stalled_queries")
	if counts[1] != 1 || counts[2] != 0 {
		t.Fatalf("超时统计结果异常: %+v", counts)
	}
	if len(queries) != 1 {
		t.Fatalf("batchStalledCounts 应只执行 1 条聚合查询，实际 %d 条", len(queries))
	}
	var steps []struct{ Detail string }
	if err := db.Raw("EXPLAIN QUERY PLAN "+queries[0].statement, queries[0].args...).Scan(&steps).Error; err != nil {
		t.Fatal(err)
	}
	indexed := false
	for _, step := range steps {
		if strings.Contains(step.Detail, "idx_episode_stalled_work") {
			indexed = true
		}
	}
	if !indexed {
		t.Fatalf("batchStalledCounts 未命中部分索引 idx_episode_stalled_work: SQL %s，计划 %v", queries[0].statement, steps)
	}
}
