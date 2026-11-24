package scheduler

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
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

type countingListDownloader struct {
	mockSchedDownloader
	listCalls int
}

func (d *countingListDownloader) List(ctx context.Context) ([]core.DownloadTask, error) {
	d.listCalls++
	return d.mockSchedDownloader.List(ctx)
}

func TestIdleDownloadPollSkipsClientAndFullTableScan(t *testing.T) {
	previous := database.DB
	if err := database.Init(filepath.Join(t.TempDir(), "download-poll.sqlite")); err != nil {
		t.Fatal(err)
	}
	db := database.DB
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); database.DB = previous })

	if err := db.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<1000)
		INSERT INTO episodes(subscription_id,number,status,torrent_hash) SELECT 1,i,'organized',printf('hash-%04d',i) FROM n`).Error; err != nil {
		t.Fatal(err)
	}

	type query struct {
		statement string
		args      []any
	}
	var queries []query
	callback := db.Callback().Query()
	if err := callback.After("gorm:query").Register("test:download_queries", func(tx *gorm.DB) {
		queries = append(queries, query{tx.Statement.SQL.String(), append([]any(nil), tx.Statement.Vars...)})
	}); err != nil {
		t.Fatal(err)
	}

	dl := &countingListDownloader{}
	s := Scheduler{downloader: dl}
	s.pollDownloads(context.Background())
	_ = callback.Remove("test:download_queries")

	if dl.listCalls != 0 {
		t.Fatalf("空载下载轮询不应请求下载器 List，实际调用 %d 次", dl.listCalls)
	}
	if len(queries) != 1 {
		t.Fatalf("空载下载轮询应只执行 1 次队列检查查询，实际 %d 次: %v", len(queries), queries)
	}
	var steps []struct{ Detail string }
	if err := db.Raw("EXPLAIN QUERY PLAN "+queries[0].statement, queries[0].args...).Scan(&steps).Error; err != nil {
		t.Fatal(err)
	}
	indexed := false
	for _, step := range steps {
		if strings.Contains(step.Detail, "idx_episode_download_work") {
			indexed = true
		}
	}
	if !indexed {
		t.Fatalf("下载轮询未命中部分索引 idx_episode_download_work: SQL %s，计划 %v", queries[0].statement, steps)
	}

	// 插入 1 条待绑定真实哈希的已整理记录，验证非空队列正常拉取并回写哈希。
	unbound := database.Episode{
		SubscriptionID: 1,
		Season:         1,
		Number:         1001,
		Title:          "Pending Bind Show - 01",
		OriginalName:   "Pending Bind Show - 01",
		Status:         "organized",
		TorrentHash:    "url:abc123",
	}
	if err := db.Create(&unbound).Error; err != nil {
		t.Fatal(err)
	}
	dl.tasks = []core.DownloadTask{{
		Hash:     "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		Name:     "Pending Bind Show - 01",
		Progress: 1,
		Status:   "uploading",
	}}
	s.pollDownloads(context.Background())
	if dl.listCalls != 1 {
		t.Fatalf("有待绑定剧集时应调用 1 次下载器 List，实际 %d 次", dl.listCalls)
	}
	var got database.Episode
	if err := db.First(&got, unbound.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.TorrentHash != "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" {
		t.Fatalf("未绑定剧集哈希未同步更新: %+v", got)
	}
}
