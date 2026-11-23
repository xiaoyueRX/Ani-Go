package scheduler

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

func TestIdleOrganizerLooksUpPendingWorkWithoutScanningHistory(t *testing.T) {
	previous := database.DB
	if err := database.Init(filepath.Join(t.TempDir(), "organizer.sqlite")); err != nil {
		t.Fatal(err)
	}
	db := database.DB
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); database.DB = previous })
	if err := db.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<1000)
		INSERT INTO episodes(subscription_id,number,status) SELECT 1,i,'organized' FROM n`).Error; err != nil {
		t.Fatal(err)
	}
	type query struct {
		statement string
		args      []any
	}
	var queries []query
	if err := db.Callback().Query().After("gorm:query").Register("test:organizer_queries", func(tx *gorm.DB) {
		queries = append(queries, query{tx.Statement.SQL.String(), append([]any(nil), tx.Statement.Vars...)})
	}); err != nil {
		t.Fatal(err)
	}
	new(Scheduler).pollOrganizer(context.Background())
	if len(queries) != 2 {
		t.Fatalf("空闲整理轮询应检查整理队列、待清理移动源，实际查询 %v", queries)
	}
	for _, q := range queries {
		var steps []struct{ Detail string }
		if err := db.Raw("EXPLAIN QUERY PLAN "+q.statement, q.args...).Scan(&steps).Error; err != nil {
			t.Fatal(err)
		}
		indexed := false
		for _, step := range steps {
			if strings.Contains(step.Detail, "idx_episode_organizer_work") {
				indexed = true
			}
		}
		if !indexed {
			t.Errorf("整理轮询应只访问待办索引: SQL %s，计划 %v", q.statement, steps)
		}
	}
}
