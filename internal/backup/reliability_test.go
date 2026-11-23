package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

func newArchiveFixture(t *testing.T, keep int) *BackupManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "nas.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	old := database.DB
	database.DB = db
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { database.DB = old; sqlDB.Close() })
	if err = db.AutoMigrate(&database.Subscription{}, &database.Episode{}, &database.Setting{}); err != nil {
		t.Fatal(err)
	}
	cfg := new(config.Config)
	cfg.Scheduler.BackupPath = t.TempDir()
	cfg.Scheduler.BackupKeepCount = keep
	return NewBackupManager(cfg)
}

func TestConcurrentArchivePublicationHonorsRetention(t *testing.T) {
	m := newArchiveFixture(t, 3)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.CreateBackup(t.Context(), true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	files, err := m.ListBackups()
	if err != nil || len(files) != 3 {
		t.Fatalf("应保留三个完整快照，实际 %d: %v", len(files), err)
	}
	for _, f := range files {
		if err := m.RestoreBackup(t.Context(), f.Name); err != nil {
			t.Fatal(err)
		}
	}
}
func TestBackupCronLifecycle(t *testing.T) {
	m := newArchiveFixture(t, 3)
	t.Setenv("BACKUP_CRON", "")
	m.cfg.Scheduler.BackupCron = "invalid"
	if err := m.Start(); err != nil {
		t.Fatalf("无效旧计划应回退到每日计划: %v", err)
	}
	t.Cleanup(m.Stop)
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if n := len(m.cron.Entries()); n != 1 {
		t.Fatalf("启动两次后有 %d 个定时任务", n)
	}
	m.Stop()
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if n := len(m.cron.Entries()); n != 1 {
		t.Fatalf("重新启动后有 %d 个定时任务", n)
	}
}
func TestRestoreKeepsEpisodeIdentityAndRejectsDuplicateIDs(t *testing.T) {
	m := newArchiveFixture(t, 7)
	sub := database.Subscription{TitleCN: "收藏"}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	ep := database.Episode{SubscriptionID: sub.ID, Number: 1, Status: "submitting", TorrentURL: "https://example.org/show", OriginalName: "第1集"}
	if err := database.DB.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	files, _ := m.ListBackups()
	if err := database.DB.Model(&ep).Updates(map[string]any{"torrent_hash": "client-assigned", "status": "downloading"}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := m.RestoreBackup(t.Context(), files[0].Name); err != nil {
			t.Fatal(err)
		}
	}
	var rows []database.Episode
	if err := database.DB.Find(&rows).Error; err != nil || len(rows) != 1 || rows[0].ID != ep.ID || rows[0].Status != "submitting" {
		t.Fatalf("重复恢复后应保留原剧集: 共 %d 条, %v", len(rows), err)
	}
	path := m.GetBackupPath(files[0].Name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var archive BackupData
	if err = json.Unmarshal(raw, &archive); err != nil {
		t.Fatal(err)
	}
	archive.FormatVersion = 0
	archive.SHA256 = ""
	archive.Episodes = append(archive.Episodes, ep)
	archive.Settings["checkpoint"] = "invalid"
	raw, _ = json.Marshal(archive)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreBackup(context.Background(), files[0].Name); err == nil {
		t.Fatalf("重复剧集编号被接受")
	}
	var n int64
	if err := database.DB.Model(&database.Setting{}).Where("key = ?", "checkpoint").Count(&n).Error; err != nil || n != 0 {
		t.Fatal("无效备份的设置写入了数据库", err)
	}
}
func TestLegacyTimestampFixture(t *testing.T) {
	m := newArchiveFixture(t, 2)
	name := "anigo_backup_20260101_120000_config.json"
	raw, _ := json.Marshal(BackupData{Timestamp: time.Now().Format(time.RFC3339), Version: "v0.5.0", Settings: map[string]string{"example": "ok"}})
	if err := os.WriteFile(filepath.Join(m.backupDir, name), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreBackup(t.Context(), name); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteArchivePreservesSeparateSourcesWithSameTitle(t *testing.T) {
	m := newArchiveFixture(t, 3)
	for _, providerID := range []string{"100", "900"} {
		sub := database.Subscription{TitleCN: "同名作品", MetadataProvider: "TMDB", MetadataID: providerID, Season: 1}
		if err := database.DB.Create(&sub).Error; err != nil {
			t.Fatal(err)
		}
		ep := database.Episode{SubscriptionID: sub.ID, Number: 1, Title: providerID}
		if err := database.DB.Create(&ep).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := m.CreateBackup(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	files, _ := m.ListBackups()
	if err := database.DB.Exec("DELETE FROM episodes").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Exec("DELETE FROM subscriptions").Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := m.RestoreBackup(t.Context(), files[0].Name); err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := database.DB.Joins("JOIN episodes ON episodes.subscription_id = subscriptions.id AND episodes.title = subscriptions.metadata_id").Model(&database.Subscription{}).Count(&count).Error; err != nil || count != 2 {
			t.Fatalf("同名条目及其剧集应分别恢复，目前 %d: %v", count, err)
		}
	}
}

func TestRestoreCreatedRowPreservesSnapshotDefaultsAndTimes(t *testing.T) {
	m := newArchiveFixture(t, 2)
	sub := database.Subscription{TitleCN: "有特殊季的作品"}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Model(&sub).Updates(map[string]any{"season": 0, "stall_timeout_hours": 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.First(&sub, sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	files, _ := m.ListBackups()
	if err := database.DB.Unscoped().Delete(&sub).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := m.RestoreBackup(t.Context(), files[0].Name); err != nil {
			t.Fatal(err)
		}
		var got database.Subscription
		if err := database.DB.First(&got, sub.ID).Error; err != nil || got.Season != 0 || got.StallTimeoutHours != 0 || !got.UpdatedAt.Equal(sub.UpdatedAt) {
			t.Fatalf("季号=%d 停滞阈值=%d 修改时间一致=%v 错误=%v", got.Season, got.StallTimeoutHours, got.UpdatedAt.Equal(sub.UpdatedAt), err)
		}
	}
}

func TestBackupsKeepHostSessionSigningKey(t *testing.T) {
	m := newArchiveFixture(t, 2)
	if err := database.DB.Save(&database.Setting{Key: "jwt_secret", Value: "local-fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	files, _ := m.ListBackups()
	path := m.GetBackupPath(files[0].Name)
	raw, _ := os.ReadFile(path)
	var b BackupData
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Settings["jwt_secret"]; ok {
		t.Error("宿主签名键出现在导出的 JSON")
	}
	b.FormatVersion = 0
	b.SHA256 = ""
	b.Settings["jwt_secret"] = "foreign-fixture"
	raw, _ = json.Marshal(b)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreBackup(t.Context(), files[0].Name); err != nil {
		t.Fatal(err)
	}
	var setting database.Setting
	if err := database.DB.First(&setting, "key = ?", "jwt_secret").Error; err != nil || setting.Value != "local-fixture" {
		t.Fatal("旧备份覆盖宿主签名键", err)
	}
}

func TestCronReloadChangesLiveScheduleAndRetention(t *testing.T) {
	m := newArchiveFixture(t, 4)
	t.Setenv("BACKUP_CRON", "")
	t.Setenv("BACKUP_KEEP_COUNT", "")
	set := func(k, v string) {
		t.Helper()
		if err := database.DB.Save(&database.Setting{Key: k, Value: v}).Error; err != nil {
			t.Fatal(err)
		}
	}
	set("BACKUP_CRON", "@every 1s")
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	if err := m.CreateBackup(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for {
		files, err := m.ListBackups()
		if err != nil {
			t.Fatal(err)
		}
		if len(files) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("定时与手动任务未分别发布快照")
		}
		time.Sleep(30 * time.Millisecond)
	}
	set("BACKUP_CRON", "0 3 * * *")
	set("BACKUP_KEEP_COUNT", "2")
	if err := m.ReloadSchedule(); err != nil {
		t.Fatal(err)
	}
	if entries := m.cron.Entries(); len(entries) != 1 || entries[0].Next.Hour() != 3 {
		t.Fatalf("五字段计划未生效: %+v", entries)
	}
	t.Setenv("BACKUP_CRON", "0 0 4 * * *")
	t.Setenv("BACKUP_KEEP_COUNT", "1")
	if err := m.ReloadSchedule(); err != nil {
		t.Fatal(err)
	}
	if entries := m.cron.Entries(); len(entries) != 1 || entries[0].Next.Hour() != 4 {
		t.Fatalf("六字段环境计划未优先: %+v", entries)
	}
	if err := m.CreateBackup(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if files, _ := m.ListBackups(); len(files) != 1 {
		t.Fatalf("热更新保留数未生效: %d", len(files))
	}
}

func TestArchiveRejectsEditedContentAndFutureFormat(t *testing.T) {
	m := newArchiveFixture(t, 3)
	if err := database.DB.Create(&database.Setting{Key: "marker", Value: "before"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	files, _ := m.ListBackups()
	path := m.GetBackupPath(files[0].Name)
	raw, _ := os.ReadFile(path)
	var original BackupData
	if err := json.Unmarshal(raw, &original); err != nil || len(original.SHA256) != 64 || original.FormatVersion != 1 {
		t.Fatal("缺少新格式和摘要", err)
	}
	for _, edit := range []func(*BackupData){func(b *BackupData) { b.Settings["marker"] = "edited" }, func(b *BackupData) { b.FormatVersion = 15 }, func(b *BackupData) { b.SHA256 = "" }} {
		var b BackupData
		if err := json.Unmarshal(raw, &b); err != nil {
			t.Fatal(err)
		}
		edit(&b)
		content, _ := json.Marshal(&b)
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
		if err := m.RestoreBackup(t.Context(), files[0].Name); !errors.Is(err, ErrInvalidArchive) {
			t.Fatal("损坏或不支持的备份未被拒绝", err)
		}
		var current database.Setting
		if err := database.DB.First(&current, "key = ?", "marker").Error; err != nil || current.Value != "before" {
			t.Fatal("校验失败后数据库被修改")
		}
	}
}

func TestArchivePreservesLabelsAndRestoresLegacyProgress(t *testing.T) {
	m := newArchiveFixture(t, 3)
	sub := database.Subscription{TitleCN: "归档", Season: 1, TotalEpisodes: 3, CurrentEpisodes: 3, Completed: true}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	rows := []database.Episode{
		{SubscriptionID: sub.ID, Season: 1, Number: 1, Status: "completed", TorrentHash: "in-client"},
		{SubscriptionID: sub.ID, Season: 1, Number: 2, Status: "completed", FinalPath: "lib/02.mkv"},
		{SubscriptionID: sub.ID, Season: 1, Number: 3, Status: "downloading", ManualStatus: "failed"},
	}
	if err := database.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	files, err := m.ListBackups()
	if err != nil || len(files) != 1 {
		t.Fatalf("归档列表：%v %v", files, err)
	}
	raw, err := os.ReadFile(m.GetBackupPath(files[0].Name))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Episodes []map[string]any `json:"episodes"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil || len(saved.Episodes) != 3 || saved.Episodes[2]["ManualStatus"] != "failed" {
		t.Fatalf("新标记未随归档保存：%v %v", saved, err)
	}
	if _, ok := saved.Episodes[0]["ManualStatus"]; ok {
		t.Fatal("旧记录应继续使用不含 ManualStatus 的旧版 JSON 形式")
	}
	if err := database.DB.Model(&rows[2]).UpdateColumn("manual_status", "completed").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := m.RestoreBackup(t.Context(), files[0].Name); err != nil {
			t.Fatal(err)
		}
		var got []database.Episode
		if err := database.DB.Order("number").Find(&got).Error; err != nil || len(got) != 3 {
			t.Fatalf("恢复剧集：%v %v", got, err)
		}
		want := [][2]string{{"downloading", "completed"}, {"organized", "completed"}, {"downloading", "failed"}}
		for i, ep := range got {
			if ep.Status != want[i][0] || ep.ManualStatus != want[i][1] {
				t.Fatalf("第 %g 集恢复为 %q/%q", ep.Number, ep.Status, ep.ManualStatus)
			}
		}
		var restored database.Subscription
		if err := database.DB.First(&restored, sub.ID).Error; err != nil || restored.CurrentEpisodes != 1 || restored.Completed {
			t.Fatalf("归档恢复后的实际进度：%+v %v", restored, err)
		}
	}
}

func TestRestorePreservesOnlyMatchingLocalFileEvidence(t *testing.T) {
	m := newArchiveFixture(t, 1)
	sub := database.Subscription{TitleCN: "多版本合集"}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	const btih = "abcdef0123456789abcdef0123456789abcdef01"
	base := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, Status: "needs_review",
		TorrentHash: btih, OriginKey: btih, DownloadRoot: "/downloads", SourceFile: "show-a.mkv", FileSize: 48, SourceModTime: 123}
	organized := base
	organized.Status, organized.SourceFile, organized.FinalPath = "organized", "show-b.mkv", "/library/b.mkv"
	organized.MoveSourcePath, organized.MoveSourceModTime = "/downloads/show-b.mkv", 456
	replaced := organized
	replaced.SourceFile, replaced.FinalPath, replaced.MoveSourcePath = "show-c.mkv", "/library/c.mkv", "/downloads/show-c.mkv"
	moved := organized
	moved.SourceFile, moved.FinalPath, moved.MoveSourcePath = "show-d.mkv", "/library/d.mkv", "/downloads/show-d.mkv"
	rows := []database.Episode{base, organized, replaced, moved}
	if err := database.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	files, err := m.ListBackups()
	if err != nil || len(files) != 1 {
		t.Fatalf("备份清单 %+v，错误 %v", files, err)
	}
	if err := database.DB.Model(&rows[0]).Updates(map[string]any{
		"torrent_hash": strings.ToUpper(btih), "origin_key": strings.ToUpper(btih),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Model(&rows[2]).Updates(map[string]any{
		"torrent_hash": "replacement", "final_path": "/library/other.mkv",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Model(&rows[3]).UpdateColumn("final_path", "/library/relocated.mkv").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := m.RestoreBackup(t.Context(), files[0].Name); err != nil {
			t.Fatal(err)
		}
		var got []database.Episode
		if err := database.DB.Order("id").Find(&got).Error; err != nil || len(got) != 4 {
			t.Fatalf("恢复后有 %d 个版本，错误 %v", len(got), err)
		}
		if got[0].Status != "needs_review" || got[0].SourceModTime != 123 {
			t.Errorf("待核对记录的本地源文件凭证丢失：%+v", got[0])
		}
		if got[1].MoveSourcePath != organized.MoveSourcePath || got[1].MoveSourceModTime != 456 {
			t.Errorf("丢失已入库记录的本地清理信息：%+v", got[1])
		}
		if got[2].SourceModTime != 0 || got[2].MoveSourcePath != "" || got[2].MoveSourceModTime != 0 {
			t.Errorf("不同文件继承了本地操作信息：%+v", got[2])
		}
		if got[3].SourceModTime != 123 || got[3].MoveSourcePath != "" || got[3].MoveSourceModTime != 0 {
			t.Errorf("目标路径变更后应只保留源文件核验时间：%+v", got[3])
		}
	}
}

func TestForeignArchivePreservesLocalSubscriptionFilters(t *testing.T) {
	m := newArchiveFixture(t, 3)
	imported := database.Subscription{TitleCN: "同一作品", Season: 1, FilterJSON: `{"resolution":"720p"}`}
	if err := database.DB.Create(&imported).Error; err != nil {
		t.Fatal(err)
	}
	episode := database.Episode{SubscriptionID: imported.ID, Number: 1, Title: "720p 资源"}
	if err := database.DB.Create(&episode).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	files, err := m.ListBackups()
	if err != nil || len(files) != 1 {
		t.Fatalf("读取备份 %v，%v", files, err)
	}
	if err := database.DB.Unscoped().Delete(&episode).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Unscoped().Delete(&imported).Error; err != nil {
		t.Fatal(err)
	}
	local := database.Subscription{TitleCN: imported.TitleCN, Season: 1, FilterJSON: `{"resolution":"1080p"}`}
	if err := database.DB.Create(&local).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := m.RestoreBackup(t.Context(), files[0].Name); err != nil {
			t.Fatal(err)
		}
	}
	var rows []database.Subscription
	if err := database.DB.Find(&rows).Error; err != nil || len(rows) != 2 {
		t.Fatalf("两条独立订阅还原后实际有 %d 条，%v", len(rows), err)
	}
	var got database.Episode
	if err := database.DB.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == local.ID && row.FilterJSON != local.FilterJSON || row.ID != local.ID && (row.ID != got.SubscriptionID || row.FilterJSON != imported.FilterJSON) {
			t.Fatalf("还原改变了筛选或剧集归属：订阅 %+v，剧集 %+v", row, got)
		}
	}
}

func TestRestoreRemappedBatchMemberAfterEpisodeAssignment(t *testing.T) {
	m := newArchiveFixture(t, 1)
	sub := database.Subscription{TitleCN: "待复核合集", Season: 1}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Status: database.EpisodeNeedsReview, ReviewReason: "请核对集号",
		TorrentHash: "abcdef0123456789abcdef0123456789abcdef01", SourceFile: "alternate.mkv", DownloadRoot: "/src", FileSize: 24}
	key := database.EpisodeFileReservationKey(sub.ID, ep.TorrentHash, ep.SourceFile)
	ep.ReservationKey = &key
	if err := database.DB.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	files, err := m.ListBackups()
	if err != nil || len(files) != 1 {
		t.Fatalf("备份清单 %+v，错误 %v", files, err)
	}
	newArchiveFixture(t, 1)
	other := database.Subscription{TitleCN: "占用原编号的作品"}
	if err := database.DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Create(&database.Episode{SubscriptionID: other.ID, Number: 1}).Error; err != nil {
		t.Fatal(err)
	}
	var restoredID uint
	for range 2 {
		if err := m.RestoreBackup(t.Context(), files[0].Name); err != nil {
			t.Fatal(err)
		}
		var rows []database.Episode
		if err := database.DB.Where("torrent_hash = ? COLLATE NOCASE", ep.TorrentHash).Find(&rows).Error; err != nil || len(rows) != 1 {
			t.Fatalf("同一合集文件恢复出 %d 条记录：%v", len(rows), err)
		}
		got := rows[0]
		want := database.EpisodeFileReservationKey(got.SubscriptionID, ep.TorrentHash, ep.SourceFile)
		if got.SubscriptionID == sub.ID || got.ID == ep.ID || got.ReservationKey == nil || *got.ReservationKey != want ||
			got.Status != database.EpisodeNeedsReview || got.ReviewReason != ep.ReviewReason || got.Number != 0 {
			t.Errorf("跨库恢复后合集成员的身份或整理状态有误：%+v", got)
		}
		if restoredID != 0 && restoredID != got.ID {
			t.Fatal("再次恢复改变了同一合集文件的编号")
		}
		restoredID = got.ID
		if err := database.DB.Model(&got).Updates(map[string]any{
			"number": 4, "status": "downloaded", "review_reason": "", "torrent_hash": strings.ToUpper(ep.TorrentHash),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestRestoreDistinguishesOpaqueTaskIDsThatDifferByCase(t *testing.T) {
	m := newArchiveFixture(t, 1)
	sub := database.Subscription{TitleCN: "不透明任务标识", Season: 1}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, TorrentHash: "Task-A", SourceFile: "episode.mkv"}
	if err := database.DB.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	files, err := m.ListBackups()
	if err != nil || len(files) != 1 {
		t.Fatalf("备份清单 %+v，错误 %v", files, err)
	}
	if err := database.DB.Unscoped().Delete(&ep).Error; err != nil {
		t.Fatal(err)
	}
	other := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, TorrentHash: "task-a", SourceFile: ep.SourceFile}
	if err := database.DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreBackup(t.Context(), files[0].Name); err != nil {
		t.Fatal(err)
	}
	var rows []database.Episode
	if err := database.DB.Order("id").Find(&rows).Error; err != nil || len(rows) != 2 ||
		rows[0].TorrentHash != ep.TorrentHash || rows[1].TorrentHash != other.TorrentHash {
		t.Fatalf("不透明任务标识被误合并：%+v %v", rows, err)
	}
}

func TestRestoreMovieProgressWithoutLocalFileTimes(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		feature      any
		want         int
		completed    bool
	}{
		{"旧归档保留显式完结状态", "organized", nil, 1, false},
		{"旧归档先恢复工作流状态", "completed", nil, 1, true},
		{"新归档明确未验证", "organized", false, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newArchiveFixture(t, 1)
			sub := database.Subscription{TitleCN: "独立电影", AnimeType: "movie", TotalEpisodes: 1, Season: 1}
			sub.ID = 24
			file := "[Group] 独立电影 [Movie].mkv"
			hash := strings.Repeat("ab", 20)
			key := database.EpisodeFileReservationKey(sub.ID, hash, file)
			// 旧版 JSON 不导出源文件修改时间。归属仍由归档中的成员键和任务名交叉验证。
			ep := map[string]any{"ID": 48, "SubscriptionID": sub.ID, "Number": 0, "Season": 1, "Status": tc.status,
				"OrganizedAt": time.Now(), "FinalPath": "/movies/movie.mkv", "SourceFile": file, "DownloadRoot": "/downloads",
				"OriginalName": strings.TrimSuffix(file, ".mkv"), "GroupName": "Group", "FileSize": 5000, "TorrentHash": hash, "ReservationKey": key}
			if tc.feature != nil {
				ep["MovieFeature"] = tc.feature
			}
			raw, err := json.Marshal(map[string]any{"timestamp": time.Now().Format(time.RFC3339), "settings": map[string]string{}, "subscriptions": []database.Subscription{sub}, "episodes": []any{ep}})
			if err != nil {
				t.Fatal(err)
			}
			const name = "anigo_backup_20260101_120000_full.json"
			if err := os.WriteFile(filepath.Join(m.cfg.Scheduler.BackupPath, name), raw, 0600); err != nil {
				t.Fatal(err)
			}
			if err := m.RestoreBackup(t.Context(), name); err != nil {
				t.Fatal(err)
			}
			var saved database.Subscription
			if err := database.DB.First(&saved).Error; err != nil {
				t.Fatal(err)
			}
			counts, err := database.FinishedEpisodeCounts(database.DB, []uint{saved.ID})
			if err != nil || counts[saved.ID] != tc.want || saved.CurrentEpisodes != tc.want || saved.Completed != tc.completed {
				t.Fatalf("restored list %v, stored %d/%d completed %t: %v", counts, saved.CurrentEpisodes, saved.TotalEpisodes, saved.Completed, err)
			}
		})
	}
}
