package database

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBootstrapNewVolumeAndCredentials(t *testing.T) {
	old := DB
	t.Cleanup(func() { DB = old })
	for _, preset := range []string{"", "configured-unique-password"} {
		t.Setenv("ANIGO_ADMIN_PASSWORD", preset)
		if err := Init(filepath.Join(t.TempDir(), "db", "anigo.db")); err != nil {
			t.Fatalf("初始化空卷: %v", err)
		}
		sqlDB, _ := DB.DB()
		t.Cleanup(func() { sqlDB.Close() })
		var seen []string
		hash := func(p string) (string, error) { seen = append(seen, p); return "hashed", nil }
		if err := InitDefaultUser(hash); err != nil {
			t.Fatal(err)
		}
		if err := InitDefaultUser(hash); err != nil {
			t.Fatal(err)
		}
		if len(seen) != 1 || (preset == "" && len(seen[0]) < 20) || (preset != "" && seen[0] != preset) {
			t.Fatal("初始化口令或重启行为错误")
		}
	}
}

func TestInitPopulatesQueryStatisticsForPopulatedDB(t *testing.T) {
	old := DB
	t.Cleanup(func() { DB = old })

	dbPath := filepath.Join(t.TempDir(), "populated.db")
	if err := Init(dbPath); err != nil {
		t.Fatalf("首次初始化数据库失败: %v", err)
	}
	firstPool, err := DB.DB()
	if err != nil {
		t.Fatal(err)
	}

	if err := DB.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<1000)
		INSERT INTO episodes(subscription_id,season,number,status,torrent_hash,origin_key,torrent_url,created_at,updated_at)
		SELECT 1,1,i,'organized',
			CASE WHEN i%2=0 THEN printf('%040X',i) ELSE printf('%040x',i) END,
			printf('%040x',i),
			printf('https://example.invalid/ep/%d.torrent',i),
			datetime('now'),datetime('now')
		FROM n`).Error; err != nil {
		_ = firstPool.Close()
		t.Fatalf("填充 episodes 测试数据失败: %v", err)
	}
	if err := DB.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<1000)
		INSERT INTO download_records(torrent_hash,torrent_url,source_name,added_at,created_at,updated_at)
		SELECT CASE WHEN i%2=0 THEN printf('%040X',i) ELSE printf('%040x',i) END,
			printf('https://example.invalid/dr/%d.torrent',i),
			'test',datetime('now'),datetime('now'),datetime('now')
		FROM n`).Error; err != nil {
		_ = firstPool.Close()
		t.Fatalf("填充 download_records 测试数据失败: %v", err)
	}
	if err := firstPool.Close(); err != nil {
		t.Fatalf("关闭首次数据库连接失败: %v", err)
	}

	// 模拟已包含历史数据的数据库重启后重新初始化。
	if err := Init(dbPath); err != nil {
		t.Fatalf("重启初始化已填充数据库失败: %v", err)
	}
	secondPool, err := DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondPool.Close() })

	var statCount int64
	if err := DB.Raw(`SELECT COUNT(*) FROM sqlite_stat1 WHERE tbl IN ('episodes', 'download_records')`).Scan(&statCount).Error; err != nil {
		t.Fatalf("查询 sqlite_stat1 失败: %v", err)
	}
	if statCount == 0 {
		t.Fatalf("Init 应在重启已填充数据库后写入 sqlite_stat1，实际记录数为 0")
	}

	probeUpper := fmt.Sprintf("%040X", 500)
	probeLower := strings.ToLower(probeUpper)
	cutoff := time.Now().Add(-24 * time.Hour)

	cases := []struct {
		name     string
		sql      string
		args     []any
		expected []string
	}{
		{
			name:     "pollSeedCleanup_hash",
			sql:      "SELECT status, final_path, organized_at FROM episodes WHERE torrent_hash = ? COLLATE NOCASE AND deleted_at IS NULL",
			args:     []any{probeLower},
			expected: []string{"idx_episode_client_hash_nocase"},
		},
		{
			name:     "isDuplicate_url",
			sql:      "SELECT count(*) FROM download_records WHERE torrent_url = ? AND deleted_at IS NULL",
			args:     []any{"https://example.invalid/dr/missing.torrent"},
			expected: []string{"idx_download_records_torrent_url"},
		},
		{
			name:     "enqueue_dedup",
			sql:      "SELECT count(*) FROM episodes WHERE (origin_key = ? OR torrent_hash = ? COLLATE NOCASE OR torrent_url = ? OR (subscription_id = ? AND season = ? AND number = ?)) AND deleted_at IS NULL",
			args:     []any{strings.Repeat("f", 40), strings.Repeat("f", 40), "https://example.invalid/ep/missing.torrent", 1, 1, 9999},
			expected: []string{"MULTI-INDEX OR"},
		},
		{
			name:     "prefetchKnownReleases_hash",
			sql:      "SELECT torrent_hash FROM episodes WHERE torrent_hash COLLATE NOCASE IN (?, ?) AND deleted_at IS NULL",
			args:     []any{probeLower, fmt.Sprintf("%040x", 501)},
			expected: []string{"idx_episode_client_hash_nocase"},
		},
		{
			name:     "pollDownloads",
			sql:      "SELECT * FROM episodes WHERE status IN ('downloading','submitting','retrying','downloaded','organized') AND (" + DownloadQueuePredicate + ") AND deleted_at IS NULL",
			expected: []string{"idx_episode_download_work"},
		},
		{
			name:     "pollOrganizer_queue",
			sql:      "SELECT * FROM episodes WHERE (" + OrganizerQueuePredicate + ") AND deleted_at IS NULL ORDER BY CASE WHEN status = 'renaming' THEN 0 ELSE 1 END",
			expected: []string{"idx_episode_organizer_work"},
		},
		{
			name:     "pollOrganizer_moves",
			sql:      "SELECT * FROM episodes WHERE status = 'organized' AND move_source_path <> '' AND deleted_at IS NULL",
			expected: []string{"idx_episode_organizer_work", "idx_episode_move_pending"},
		},
		{
			name:     "batchStalledCounts",
			sql:      "SELECT subscription_id, count(*) as count FROM episodes WHERE subscription_id IN (1, 2) AND " + StalledEpisodePredicate + " AND ((status = 'pending' AND created_at < ?) OR (status = 'downloading' AND download_started_at < ?)) AND deleted_at IS NULL GROUP BY subscription_id",
			args:     []any{cutoff, cutoff},
			expected: []string{"idx_episode_stalled_work"},
		},
	}

	for _, tc := range cases {
		var steps []struct{ Detail string }
		if err := DB.Raw("EXPLAIN QUERY PLAN "+tc.sql, tc.args...).Scan(&steps).Error; err != nil {
			t.Fatalf("%s EXPLAIN QUERY PLAN 失败: %v", tc.name, err)
		}
		var details []string
		matched := false
		for _, step := range steps {
			details = append(details, step.Detail)
			for _, want := range tc.expected {
				if strings.Contains(step.Detail, want) {
					matched = true
				}
			}
		}
		if !matched {
			t.Fatalf("%s 未命中预期索引 %v，实际查询计划: %v", tc.name, tc.expected, details)
		}
	}
}
