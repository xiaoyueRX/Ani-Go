package database

import (
	"path/filepath"
	"testing"
)

func TestLegacyEpisodeReviewUpgrade(t *testing.T) {
	prev := DB
	t.Cleanup(func() { DB = prev })
	path := filepath.Join(t.TempDir(), "anigo.db")
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	ep := Episode{SubscriptionID: 1, Status: "downloaded", OriginalName: "没有集数.mkv"}
	if err := DB.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec("ALTER TABLE episodes DROP COLUMN review_reason").Error; err != nil {
		t.Fatal(err)
	}
	conn, err := DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	conn, err = DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	var got Episode
	if err := DB.First(&got, ep.ID).Error; err != nil || got.Status != "downloaded" || got.ReviewReason != "" || !got.UpdatedAt.Equal(ep.UpdatedAt) {
		t.Fatalf("旧记录迁移改变了调度状态：%+v %v", got, err)
	}
	if changed, err := MarkEpisodeForReview(t.Context(), got, "   "); err == nil || changed {
		t.Fatalf("空原因被接受：%v %v", changed, err)
	}
	if changed, err := MarkEpisodeForReview(t.Context(), got, "未识别"); err != nil || !changed {
		t.Fatalf("旧记录无法复核：%v %v", changed, err)
	}
	if err := DB.First(&got, ep.ID).Error; err != nil || got.ReviewReason != "未识别" || got.Status != "needs_review" {
		t.Fatalf("复核未持久化：%+v %v", got, err)
	}
}

func TestOrdinaryReviewPreservesPreviouslyConfirmedSource(t *testing.T) {
	prev := DB
	t.Cleanup(func() { DB = prev })
	if err := Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	ep := Episode{Status: "downloaded", SubscriptionID: 1, TorrentHash: "task", SourceFile: "episode.mkv", DownloadRoot: "/data", SourceModTime: 5, FileSize: 10, Number: 4, Season: 1}
	ep.ConfirmedIdentity = ep.ConfirmationSignature(ReviewEpisodeIdentity)
	if err := DB.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := MarkEpisodeForReview(t.Context(), ep, "目标文件冲突"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	var got Episode
	if err := DB.First(&got, ep.ID).Error; err != nil || !got.HasConfirmedIdentity(ReviewEpisodeIdentity, 10, 5) || got.HasConfirmedIdentity(ReviewEpisodeIdentity, 10, 6) {
		t.Fatalf("普通冲突丢失了之前的源身份校验：%+v %v", got, err)
	}
}

func TestReviewTaskIdentityPreservesOpaqueCase(t *testing.T) {
	a := Episode{SubscriptionID: 1, TorrentHash: "Task-A", SourceFile: "01.mkv", DownloadRoot: "/data", FileSize: 1, SourceModTime: 2, Number: 1, Season: 1}
	b := a
	b.TorrentHash = "task-a"
	if EpisodeFileReservationKey(1, a.TorrentHash, a.SourceFile) == EpisodeFileReservationKey(1, b.TorrentHash, b.SourceFile) || a.ConfirmationSignature(ReviewEpisodeIdentity) == b.ConfirmationSignature(ReviewEpisodeIdentity) {
		t.Fatal("不同的下载器任务共享了文件预留键或人工确认")
	}
}
