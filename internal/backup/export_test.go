package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

func TestExportUsesOneSnapshotAcrossEpisodeBatches(t *testing.T) {
	m := newArchiveFixture(t, 4)
	const original = "旧收藏 <星&海> 🌙\u2028第二季"
	sub := database.Subscription{TitleCN: original}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	const n = 513
	episodes := make([]database.Episode, n)
	for i := range episodes {
		episodes[i] = database.Episode{SubscriptionID: sub.ID, Number: float32(i + 1), Title: original}
	}
	if err := database.DB.CreateInBatches(&episodes, 20).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatal(err)
	}
	pool, _ := database.DB.DB()
	pool.SetMaxOpenConns(2)
	var batches int
	if err := database.DB.Callback().Query().After("gorm:query").Register("test:concurrent_export_write", func(tx *gorm.DB) {
		if tx.Statement.Table != "episodes" || tx.RowsAffected == 0 {
			return
		}
		batches++
		if tx.RowsAffected >= n {
			t.Errorf("导出一次载入全部 %d 个剧集", n)
		}
		if batches == 1 {
			err := database.DB.Transaction(func(writer *gorm.DB) error {
				if err := writer.Model(&sub).Update("title_cn", "新收藏").Error; err != nil {
					return err
				}
				return writer.Model(&episodes[n-1]).Update("title", "新收藏").Error
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if batches < 2 {
		t.Errorf("只读取了 %d 批剧集", batches)
	}
	files, err := m.ListBackups()
	if err != nil || len(files) != 1 {
		t.Fatalf("备份清单 %v, %v", files, err)
	}
	raw, err := os.ReadFile(filepath.Join(m.backupDir, files[0].Name))
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyArchive(raw); err != nil {
		t.Fatal(err)
	}
	var archive BackupData
	if err := json.Unmarshal(raw, &archive); err != nil {
		t.Fatal(err)
	}
	if archive.FormatVersion != 1 || len(archive.Subscriptions) != 1 || len(archive.Episodes) != n || archive.Subscriptions[0].TitleCN != original {
		t.Fatalf("快照有误: 版本 %d，订阅 %d，剧集 %d", archive.FormatVersion, len(archive.Subscriptions), len(archive.Episodes))
	}
	for i, row := range archive.Episodes {
		if row.ID != episodes[i].ID || row.Title != original {
			t.Fatalf("剧集 %d 未来自同一快照: ID %d，标题 %q", i, row.ID, row.Title)
		}
	}
}

func TestExportCancellationDiscardsPartialArchive(t *testing.T) {
	m := newArchiveFixture(t, 2)
	sub := database.Subscription{TitleCN: "待取消"}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	episodes := make([]database.Episode, 513)
	for i := range episodes {
		episodes[i] = database.Episode{SubscriptionID: sub.ID, Number: float32(i + 1)}
	}
	if err := database.DB.CreateInBatches(&episodes, 20).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := database.DB.Callback().Query().After("gorm:query").Register("test:cancel_export", func(tx *gorm.DB) {
		if tx.Statement.Table == "episodes" {
			cancel()
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(ctx, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消返回 %v", err)
	}
	files, err := os.ReadDir(m.backupDir)
	if err != nil || len(files) != 0 {
		t.Fatalf("取消后仍有备份或临时文件: %v, %v", files, err)
	}
}

func TestExportEmptyArrays(t *testing.T) {
	m := newArchiveFixture(t, 2)
	for _, include := range []bool{false, true} {
		if err := m.CreateBackup(t.Context(), include); err != nil {
			t.Fatal(err)
		}
	}
	files, err := m.ListBackups()
	if err != nil || len(files) != 2 {
		t.Fatalf("备份清单 %v, %v", files, err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(m.backupDir, f.Name))
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields["episodes"]; ok || string(fields["subscriptions"]) != "[]" || verifyArchive(raw) != nil {
			t.Fatalf("空库存档格式有误: %s", raw)
		}
	}
}
