package downloader

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type mockDownloader struct {
	tasks       []core.DownloadTask
	deleted     []string
	deleteFiles []bool
}

func (m *mockDownloader) Name() string { return "mock" }
func (m *mockDownloader) Add(ctx context.Context, item core.TorrentItem, savePath string) error {
	return nil
}
func (m *mockDownloader) List(ctx context.Context) ([]core.DownloadTask, error) {
	return m.tasks, nil
}
func (m *mockDownloader) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	for _, t := range m.tasks {
		if t.Hash == hash {
			return t, nil
		}
	}
	return core.DownloadTask{}, nil
}
func (m *mockDownloader) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	m.deleted = append(m.deleted, hash)
	m.deleteFiles = append(m.deleteFiles, deleteFiles)
	return nil
}
func (m *mockDownloader) Pause(ctx context.Context, hash string) error  { return nil }
func (m *mockDownloader) Resume(ctx context.Context, hash string) error { return nil }
func (m *mockDownloader) IsAvailable(ctx context.Context) bool          { return true }

func TestCleanSatisfiedSeeds(t *testing.T) {
	ctx := context.Background()

	mock := &mockDownloader{
		tasks: []core.DownloadTask{
			{
				Hash:        "hash-downloading",
				Name:        "task-downloading",
				Progress:    0.5,
				Ratio:       2.0,
				SeedingTime: 1000,
			},
			{
				Hash:        "hash-seeding-not-enough-time",
				Name:        "task-not-enough-time",
				Progress:    1.0,
				Ratio:       2.0,
				SeedingTime: 50, // < 100s
			},
			{
				Hash:        "hash-seeding-not-enough-ratio",
				Name:        "task-not-enough-ratio",
				Progress:    1.0,
				Ratio:       0.5, // < 1.0
				SeedingTime: 200,
			},
			{
				Hash:        "hash-protected-by-filter",
				Name:        "task-protected",
				Progress:    1.0,
				Ratio:       2.0,
				SeedingTime: 200,
			},
			{
				Hash:        "hash-satisfied",
				Name:        "task-satisfied",
				Progress:    1.0,
				Ratio:       2.0,
				SeedingTime: 200,
			},
		},
	}

	canDelete := func(task core.DownloadTask) bool {
		return task.Hash != "hash-protected-by-filter"
	}

	cleaned, err := CleanSatisfiedSeeds(ctx, mock, 100*time.Second, 1.0, canDelete)
	if err != nil {
		t.Fatalf("CleanSatisfiedSeeds 失败: %v", err)
	}

	if cleaned != 1 {
		t.Fatalf("期望清理 1 个任务，实际清理 %d 个", cleaned)
	}

	if len(mock.deleted) != 1 || mock.deleted[0] != "hash-satisfied" {
		t.Errorf("删除的任务期望 [hash-satisfied], 实际 %v", mock.deleted)
	}

	// 必须严格确保 deleteFiles 为 false（绝不删物理文件，仅删做种记录）
	if len(mock.deleteFiles) != 1 || mock.deleteFiles[0] != false {
		t.Errorf("deleteFiles 期望 false, 实际 %v", mock.deleteFiles)
	}
}
