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

func TestCleanSatisfiedSeeds_NilDownloader(t *testing.T) {
	_, err := CleanSatisfiedSeeds(context.Background(), nil, time.Hour, 1.0, nil)
	if err == nil {
		t.Fatal("期望 nil downloader 报错，但返回 nil")
	}
}

func TestCleanSatisfiedSeeds_DisabledThresholds(t *testing.T) {
	mock := &mockDownloader{
		tasks: []core.DownloadTask{
			{Hash: "h1", Progress: 1.0, Ratio: 5.0, SeedingTime: 99999},
		},
	}
	// 当 minSeedTime <= 0 且 minRatio <= 0 时，自动跳过清理以防全量误删
	cleaned, err := CleanSatisfiedSeeds(context.Background(), mock, 0, 0, nil)
	if err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if cleaned != 0 || len(mock.deleted) != 0 {
		t.Errorf("阈值均 <= 0 时不应清理任何任务，实际清理了 %d 个", cleaned)
	}
}

func TestCleanSatisfiedSeeds_ZeroSeedingTimeProtection(t *testing.T) {
	// 刚完成的种子（SeedingTime = 0）在设置了 minSeedTime 时决不能被删除
	mock := &mockDownloader{
		tasks: []core.DownloadTask{
			{
				Hash:        "just-finished",
				Name:        "just-finished-task",
				Progress:    1.0,
				Ratio:       0.0,
				SeedingTime: 0,
			},
		},
	}

	cleaned, err := CleanSatisfiedSeeds(context.Background(), mock, 24*time.Hour, 0, nil)
	if err != nil {
		t.Fatalf("CleanSatisfiedSeeds 失败: %v", err)
	}
	if cleaned != 0 {
		t.Errorf("刚完成做种时间为 0 的任务不应被清理，实际清理了 %d 个", cleaned)
	}
}

func TestIsTaskReadyForSeedCleanup(t *testing.T) {
	cases := []struct {
		name     string
		progress float32
		status   string
		expected bool
	}{
		{"未完成 50%", 0.5, "downloading", false},
		{"未完成 99%", 0.99, "downloading", false},
		{"完成但正在校验", 1.0, "checking", false},
		{"完成但下载异常", 1.0, "error", false},
		{"完成但文件丢失", 1.0, "missingFiles", false},
		{"做种上传中", 1.0, "uploading", true},
		{"做种无流量", 1.0, "stalledUP", true},
		{"做种暂停", 1.0, "pausedUP", true},
		{"Transmission 做种中", 1.0, "seeding", true},
		{"Transmission 已停止", 1.0, "stopped", true},
		{"Aria2 complete 规范化", 1.0, "completed", true},
		{"单测空状态放行", 1.0, "", true},
	}

	for _, tc := range cases {
		task := core.DownloadTask{
			Progress: tc.progress,
			Status:   tc.status,
		}
		got := isTaskReadyForSeedCleanup(task)
		if got != tc.expected {
			t.Errorf("[%s] 期望 %v, 得到 %v", tc.name, tc.expected, got)
		}
	}
}
