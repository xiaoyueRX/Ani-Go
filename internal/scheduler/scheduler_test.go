package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

func TestBuildFilter_FromSubscription(t *testing.T) {
	sub := database.Subscription{
		SubgroupName: "千夏字幕组",
		FilterJSON:   `{"include_keywords":["1080p","CHS"],"exclude_keywords":["BIG5"],"resolution":"1080p"}`,
	}
	filter := buildFilter(sub)
	if filter.PreferSubgroup != "千夏字幕组" {
		t.Errorf("PreferSubgroup = %q, 期望 %q", filter.PreferSubgroup, "千夏字幕组")
	}
	if len(filter.IncludeKeywords) != 2 {
		t.Errorf("IncludeKeywords 数量 = %d, 期望 2", len(filter.IncludeKeywords))
	}
	if filter.IncludeKeywords[0] != "1080p" {
		t.Errorf("IncludeKeywords[0] = %q, 期望 %q", filter.IncludeKeywords[0], "1080p")
	}
	if filter.Resolution != "1080p" {
		t.Errorf("Resolution = %q, 期望 %q", filter.Resolution, "1080p")
	}
}

func TestBuildFilter_NoFilterJSON(t *testing.T) {
	sub := database.Subscription{
		SubgroupName: "喵萌奶茶屋",
	}
	filter := buildFilter(sub)
	if filter.PreferSubgroup != "喵萌奶茶屋" {
		t.Errorf("PreferSubgroup = %q, 期望 %q", filter.PreferSubgroup, "喵萌奶茶屋")
	}
	if len(filter.IncludeKeywords) != 0 {
		t.Error("无 FilterJSON 时 IncludeKeywords 应为空")
	}
}

func TestBuildFilter_InvalidJSON(t *testing.T) {
	sub := database.Subscription{
		FilterJSON: "not valid json!!!",
	}
	// 不应 panic
	filter := buildFilter(sub)
	if filter.PreferSubgroup != "" {
		t.Error("无 SubgroupName 时 PreferSubgroup 应为空")
	}
}

// 确保 buildFilter 返回的 Filter 可以用在 Mikan Source 上
func TestBuildFilter_ReturnsValidFilter(t *testing.T) {
	sub := database.Subscription{
		SubgroupName: "桜都字幕组",
		FilterJSON:   `{"include_keywords":["1080p"],"exclude_keywords":["720p"]}`,
	}
	filter := buildFilter(sub)
	// 类型检查
	var _ core.Filter = filter
	if filter.PreferSubgroup != "桜都字幕组" {
		t.Errorf("PreferSubgroup 不匹配")
	}
}

func TestMatchSubscriptionFilter(t *testing.T) {
	// 1. 排除关键词测试
	filter1 := core.Filter{
		ExcludeKeywords: []string{"720p", "BIG5"},
	}
	if matchSubscriptionFilter(core.TorrentItem{Title: "[ANi] 芙莉莲 - 01 [1080p]"}, filter1) != true {
		t.Errorf("不包含排除词的条目应放行")
	}
	if matchSubscriptionFilter(core.TorrentItem{Title: "[ANi] 芙莉莲 - 01 [720p]"}, filter1) != false {
		t.Errorf("包含 720p 的条目应被排除拦截")
	}
	if matchSubscriptionFilter(core.TorrentItem{Title: "[ANi] 芙莉莲 - 01 [BIG5]"}, filter1) != false {
		t.Errorf("包含 BIG5 的条目应被排除拦截")
	}

	// 2. 包含关键词测试（必须至少命中一个）
	filter2 := core.Filter{
		IncludeKeywords: []string{"CHS", "简日"},
	}
	if matchSubscriptionFilter(core.TorrentItem{Title: "[ANi] 芙莉莲 - 01 [CHS]"}, filter2) != true {
		t.Errorf("包含 CHS 的条目应放行")
	}
	if matchSubscriptionFilter(core.TorrentItem{Title: "[ANi] 芙莉莲 - 01 [简日双语]"}, filter2) != true {
		t.Errorf("包含 简日 的条目应放行")
	}
	if matchSubscriptionFilter(core.TorrentItem{Title: "[ANi] 芙莉莲 - 01 [繁体中文]"}, filter2) != false {
		t.Errorf("不包含指定包含词的条目应被拦截")
	}

	// 3. 分辨率测试
	filter3 := core.Filter{
		Resolution: "1080p",
	}
	if matchSubscriptionFilter(core.TorrentItem{Title: "Anime Title 1080P"}, filter3) != true {
		t.Errorf("大小写不敏感 1080P 应放行")
	}
	if matchSubscriptionFilter(core.TorrentItem{Title: "Anime Title 2160P"}, filter3) != false {
		t.Errorf("非 1080p 分辨率应被拦截")
	}

	// 4. 允许字幕组列表
	filter4 := core.Filter{
		AllowedSubgroups: []string{"喵萌奶茶屋", "千夏字幕组"},
	}
	if matchSubscriptionFilter(core.TorrentItem{Title: "[喵萌奶茶屋] Anime - 01", GroupName: "喵萌奶茶屋"}, filter4) != true {
		t.Errorf("在允许列表内的字幕组应放行")
	}
	if matchSubscriptionFilter(core.TorrentItem{Title: "[极影字幕社] Anime - 01", GroupName: "极影字幕社"}, filter4) != false {
		t.Errorf("不在允许列表内的字幕组应被拦截")
	}
}

func TestPollSeedCleanup_StrictGuard(t *testing.T) {
	// 初始化内存 SQLite 测试环境
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("初始化内存数据库失败: %v", err)
	}
	_ = db.AutoMigrate(&database.Subscription{}, &database.Episode{}, &database.Setting{})
	origDB := database.DB
	database.DB = db
	defer func() { database.DB = origDB }()

	now := time.Now()

	// 1. 还在下载中的种子
	database.DB.Create(&database.Episode{
		SubscriptionID: 1,
		Season:         1,
		Number:         1,
		Title:          "Ep1-Downloading",
		TorrentHash:    "hash-downloading",
		Status:         "downloading",
	})

	// 2. 状态为 organized，但 FinalPath 为空的种子（未真正规范整理）
	database.DB.Create(&database.Episode{
		SubscriptionID: 1,
		Season:         1,
		Number:         2,
		Title:          "Ep2-EmptyFinalPath",
		TorrentHash:    "hash-empty-finalpath",
		Status:         "organized",
		FinalPath:      "",
		OrganizedAt:    &now,
	})

	// 3. 状态为 organized，但 OrganizedAt 为 nil 的种子
	database.DB.Create(&database.Episode{
		SubscriptionID: 1,
		Season:         1,
		Number:         3,
		Title:          "Ep3-NilOrganizedAt",
		TorrentHash:    "hash-nil-organizedat",
		Status:         "organized",
		FinalPath:      "/media/tv/ep3.mkv",
		OrganizedAt:    nil,
	})

	// 4. 严格规范整理完成的合法种子
	database.DB.Create(&database.Episode{
		SubscriptionID: 1,
		Season:         1,
		Number:         4,
		Title:          "Ep4-ProperlyOrganized",
		TorrentHash:    "hash-properly-organized",
		Status:         "organized",
		FinalPath:      "/media/tv/ep4.mkv",
		OrganizedAt:    &now,
	})

	mock := &mockSchedDownloader{
		tasks: []core.DownloadTask{
			{Hash: "hash-downloading", Progress: 1.0, Ratio: 3.0, SeedingTime: 300000},
			{Hash: "hash-empty-finalpath", Progress: 1.0, Ratio: 3.0, SeedingTime: 300000},
			{Hash: "hash-nil-organizedat", Progress: 1.0, Ratio: 3.0, SeedingTime: 300000},
			{Hash: "hash-properly-organized", Progress: 1.0, Ratio: 3.0, SeedingTime: 300000},
			{Hash: "hash-unmanaged-external", Progress: 1.0, Ratio: 5.0, SeedingTime: 300000}, // 用户自下种子，不在 DB 中
		},
	}

	sched := &Scheduler{
		downloader: mock,
	}

	sched.pollSeedCleanup(context.Background())

	// 校验：仅有 hash-properly-organized 允许被删除，其他 4 个严禁被删除！
	if len(mock.deleted) != 1 {
		t.Fatalf("严格做种保护期望仅清理 1 个任务，实际清理了 %d 个: %v", len(mock.deleted), mock.deleted)
	}
	if mock.deleted[0] != "hash-properly-organized" {
		t.Errorf("期望删除合法整理完毕的 hash-properly-organized, 实际删除: %s", mock.deleted[0])
	}
	// 验证必须传入 deleteFiles=false
	if len(mock.deleteFiles) != 1 || mock.deleteFiles[0] != false {
		t.Errorf("必须强制 deleteFiles=false 保留本地硬链接物理文件, 实际: %v", mock.deleteFiles)
	}
}

func TestScheduler_NilDBGuards(t *testing.T) {
	origDB := database.DB
	database.DB = nil
	defer func() { database.DB = origDB }()

	// 测试当 database.DB == nil 时，所有底层辅助函数不发生 panic
	if isDuplicate("https://example.com/test.torrent") != false {
		t.Error("nil DB 下 isDuplicate 应返回 false")
	}
	if episodeNumberExists(1, 1, 1) != false {
		t.Error("nil DB 下 episodeNumberExists 应返回 false")
	}
	if isEpisodeExists("abc") != false {
		t.Error("nil DB 下 isEpisodeExists 应返回 false")
	}
	if episodeGroupExists(1, 1, 1, "group") != false {
		t.Error("nil DB 下 episodeGroupExists 应返回 false")
	}

	// 写入函数静默安全返回
	recordDownload(core.TorrentItem{Title: "Test", URL: "http://test"})
	createEpisodeRecord(1, core.TorrentItem{Title: "Test"})
	createEpisodeRecordWithParsed(1, core.TorrentItem{Title: "Test"}, 1, 1)

	// pollSyncBangumi / autoSubscribe 安全返回错误而非 panic
	sched := &Scheduler{}
	sched.pollSyncBangumi(context.Background())
	_, err := sched.autoSubscribe(context.Background(), core.Anime{ID: "123"})
	if err == nil {
		t.Error("nil DB 下 autoSubscribe 应返回错误")
	}
}

type mockSchedDownloader struct {
	tasks       []core.DownloadTask
	deleted     []string
	deleteFiles []bool
}

func (m *mockSchedDownloader) Name() string { return "mock" }
func (m *mockSchedDownloader) Add(ctx context.Context, item core.TorrentItem, savePath string) error {
	return nil
}
func (m *mockSchedDownloader) List(ctx context.Context) ([]core.DownloadTask, error) {
	return m.tasks, nil
}
func (m *mockSchedDownloader) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	return core.DownloadTask{}, nil
}
func (m *mockSchedDownloader) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	m.deleted = append(m.deleted, hash)
	m.deleteFiles = append(m.deleteFiles, deleteFiles)
	return nil
}
func (m *mockSchedDownloader) Pause(ctx context.Context, hash string) error  { return nil }
func (m *mockSchedDownloader) Resume(ctx context.Context, hash string) error { return nil }
func (m *mockSchedDownloader) IsAvailable(ctx context.Context) bool          { return true }
