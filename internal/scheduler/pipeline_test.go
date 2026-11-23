package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
	"gorm.io/gorm"
)

type pipelineSource struct {
	core.Source
	rss, history []core.TorrentItem
}

func (s pipelineSource) FetchRSS(context.Context, string) ([]core.TorrentItem, error) {
	return s.rss, nil
}
func (s pipelineSource) FetchHistory(context.Context, string, core.Filter) ([]core.TorrentItem, error) {
	return s.history, nil
}

type pipelineDownloader struct {
	mockSchedDownloader
	calls   atomic.Int32
	started chan struct{}
	resume  chan struct{}
}

func (d *pipelineDownloader) Add(ctx context.Context, item core.TorrentItem, dir string) error {
	if d.calls.Add(1) == 1 && d.started != nil {
		close(d.started)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.resume:
		}
	}
	return nil
}
func (d *pipelineDownloader) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	for _, task := range d.tasks {
		if task.Hash == hash {
			return task, nil
		}
	}
	return core.DownloadTask{}, os.ErrNotExist
}

func pipelineDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fixture.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&database.Subscription{}, &database.Episode{}, &database.DownloadRecord{}, &database.Setting{}, &database.ParserCache{}); err != nil {
		t.Fatal(err)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old; _ = conn.Close() })
	return db
}

func savePipelineRow(t *testing.T, db *gorm.DB, row any) {
	t.Helper()
	if err := db.Create(row).Error; err != nil {
		t.Fatal(err)
	}
}

func TestCompletionBindsLegacyPlaceholder(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series"}
	savePipelineRow(t, db, &sub)
	ep := database.Episode{SubscriptionID: sub.ID, Number: 1, Title: "Series - 01", OriginalName: "Series - 01", TorrentHash: "url:11111111111111111111111111111111", Status: "downloading"}
	savePipelineRow(t, db, &ep)
	down := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{tasks: []core.DownloadTask{{Hash: "abcdef0123456789abcdef0123456789abcdef01", Name: ep.OriginalName, Progress: 1, Status: "completed"}}}}
	s := Scheduler{downloader: down}
	s.pollDownloads(context.Background())
	if err := db.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ep.Status != "downloaded" || ep.TorrentHash != down.tasks[0].Hash || ep.DownloadFinishedAt == nil {
		t.Fatalf("完成任务没有关联到剧集: %+v", ep)
	}
}

func TestOrganizerLeavesPartialDownloadUntouched(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", AnimeType: "tv"}
	savePipelineRow(t, db, &sub)
	src := filepath.Join(t.TempDir(), "partial.mkv")
	if err := os.WriteFile(src, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, TorrentHash: "hash", Status: "downloading"}
	savePipelineRow(t, db, &ep)
	down := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{tasks: []core.DownloadTask{{Hash: "hash", ContentPath: src, Progress: 0.2, Status: "downloading"}}}}
	s := Scheduler{downloader: down, organizer: organizer.New("{title_cn}/S{season:02}E{ep:02}", "", "", t.TempDir(), "", false, nil)}
	s.pollOrganizer(context.Background())
	if err := db.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil || ep.Status != "downloading" || ep.FinalPath != "" {
		t.Fatalf("未完成文件被整理: %v %+v", err, ep)
	}
}

func TestSupplementKeepsSubscriptionAndSpecialSeasons(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", Season: 2, BangumiID: "1234"}
	savePipelineRow(t, db, &sub)
	src := pipelineSource{history: []core.TorrentItem{{Title: "Series - 05 [1080p]", URL: "test:five", InfoHash: "hash5"}, {Title: "Series SP01 [1080p]", URL: "test:special", InfoHash: "hashSP"}, {Title: "Series S01E03 [1080p]", URL: "test:three", InfoHash: "hash3"}}}
	s := Scheduler{cfg: &config.Config{}, source: src, downloader: &pipelineDownloader{}}
	s.supplementOne(context.Background(), sub)
	for hash, season := range map[string]int{"hash5": 2, "hashSP": 0} {
		var ep database.Episode
		if err := db.Where("torrent_hash = ?", hash).First(&ep).Error; err != nil {
			t.Fatal(err)
		}
		if ep.Season != season {
			t.Errorf("%s 季号 %d，期望 %d", hash, ep.Season, season)
		}
	}
	var count int64
	if err := db.Model(&database.Episode{}).Where("torrent_hash = ?", "hash3").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("跨季条目未被过滤: %d %v", count, err)
	}
	if season, num := s.resolveSubscriptionEpisode(context.Background(), src.history[2], sub); season != 1 || num != 3 {
		t.Fatalf("显式季号被改写: %d/%g", season, num)
	}

}

func TestBatchOrganizesEachEpisodeFile(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", Season: 1, AnimeType: "tv", BangumiID: "1234"}
	savePipelineRow(t, db, &sub)
	item := core.TorrentItem{Title: "Series [01-02] [1080p]", URL: "test:batch", InfoHash: "hash"}
	dir := t.TempDir()
	lib := t.TempDir()
	for _, file := range []string{"S01E01.mkv", "S01E02.mkv"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0600); err != nil {
			t.Fatal(err)
		}
	}
	down := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{tasks: []core.DownloadTask{{Hash: "hash", Name: item.Title, ContentPath: dir, Progress: 1, Status: "completed"}}}}
	s := Scheduler{cfg: &config.Config{}, source: pipelineSource{history: []core.TorrentItem{item}}, downloader: down, organizer: organizer.New("{title_cn}/S{season:02}E{ep:02}", "", "", lib, "", true, nil)}
	s.supplementOne(context.Background(), sub)
	s.pollDownloads(context.Background())
	s.pollOrganizer(context.Background())
	s.pollOrganizer(context.Background())
	var rows []database.Episode
	if err := db.Where("status = ?", "organized").Order("number").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("合集整理后应得到两集: %+v", rows)
	}
	for i, ep := range rows {
		content, err := os.ReadFile(ep.FinalPath)
		if err != nil || ep.Number != float32(i+1) || string(content) != filepath.Base(ep.FinalPath) {
			t.Fatalf("集与文件不匹配: %+v %q %v", ep, content, err)
		}
	}
}

func TestOverlappingRSSAndSupplementSubmitEpisodeOnce(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", Season: 1, BangumiID: "1234"}
	savePipelineRow(t, db, &sub)
	src := pipelineSource{rss: []core.TorrentItem{{Title: "Series - 05 [1080p]", URL: "test:A", InfoHash: "hashA"}}, history: []core.TorrentItem{{Title: "Series - 05 [720p]", URL: "test:B", InfoHash: "hashB"}}}
	down := &pipelineDownloader{started: make(chan struct{}), resume: make(chan struct{})}
	s := Scheduler{cfg: &config.Config{}, source: src, mikanRSSURL: "test:rss", downloader: down}
	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { defer close(done); s.pollRSS(ctx) }()
	select {
	case <-down.started:
	case <-ctx.Done():
		t.Fatal("RSS 未投递")
	}
	suppDone := make(chan struct{})
	go func() { defer close(suppDone); s.supplementOne(ctx, sub) }()
	select {
	case <-suppDone:
	case <-ctx.Done():
		close(down.resume)
		<-done
		t.Fatal("补全等待占用中的下载器调用")
	}
	close(down.resume)
	<-done
	var count int64
	db.Model(&database.Episode{}).Where("number = 5").Count(&count)
	if down.calls.Load() != 1 || count != 1 {
		t.Fatalf("预期同集一次投递，得到 %d 次、%d 条记录", down.calls.Load(), count)
	}
}

func TestReconcileLegacyIdentityAndSeasons(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", Season: 2}
	savePipelineRow(t, db, &sub)
	special := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, OriginalName: "Series SP01", Status: "downloading", TorrentHash: "url:0123456789abcdef0123456789abcdef"}
	savePipelineRow(t, db, &special)
	ordinary := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 5, OriginalName: "Series - 05", Status: "downloading", TorrentHash: "hash5"}
	savePipelineRow(t, db, &ordinary)
	s := Scheduler{}
	if err := s.repairLegacyEpisodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.repairLegacyEpisodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&special, special.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&ordinary, ordinary.ID).Error; err != nil {
		t.Fatal(err)
	}
	if special.Season != 0 || special.TorrentHash != "" || special.OriginKey == "" || ordinary.Season != 2 {
		t.Fatalf("旧记录未正确升级: %+v %+v", special, ordinary)
	}
}

func TestCompletionRecoversInterruptedSubmission(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series"}
	savePipelineRow(t, db, &sub)
	ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 2, OriginalName: "Series - 02", Title: "Series - 02", Status: "submitting", OriginKey: "url:abc", TorrentURL: "test:url", TorrentHash: "torrent"}
	savePipelineRow(t, db, &ep)
	if err := db.Model(&ep).UpdateColumn("updated_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	down := &pipelineDownloader{}
	s := Scheduler{downloader: down}
	s.pollDownloads(context.Background())
	if err := db.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ep.Status != "downloading" || down.calls.Load() != 1 {
		t.Fatalf("中断的投递没有恢复: %+v %d", ep, down.calls.Load())
	}
}

func TestCompletionDoesNotGuessAmbiguousNames(t *testing.T) {
	db := pipelineDB(t)
	ep := database.Episode{SubscriptionID: 1, Number: 1, OriginalName: "same", Status: "downloading"}
	savePipelineRow(t, db, &ep)
	down := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{tasks: []core.DownloadTask{{Hash: "a", Name: "same", Progress: 1, Status: "completed"}, {Hash: "b", Name: "same", Progress: 1, Status: "completed"}}}}
	s := Scheduler{downloader: down}
	s.pollDownloads(context.Background())
	if err := db.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ep.Status != "downloading" || ep.TorrentHash != "" {
		t.Fatalf("同名任务被误认: %+v", ep)
	}
}

func TestClassicCreatesFromWrappedMikanWithoutCredentials(t *testing.T) {
	db := pipelineDB(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<button class="js-subscribe_bangumi_page" data-bangumiid="51"></button>`)
	}))
	defer server.Close()
	save := httpx.Default
	httpx.Default = server.Client()
	defer func() { httpx.Default = save }()
	src := source.NewMultiSource(source.NewMikanSource(strings.TrimPrefix(server.URL, "https://"), "", nil))
	s := Scheduler{source: src}
	item := core.TorrentItem{Title: "Fixture - 01", EpisodeURL: server.URL + "/Home/Episode/release"}
	id, err := autoCreateSubscription(context.Background(), &s, item)
	if err != nil || id == 0 {
		t.Fatalf("经典订阅失败: %d %v", id, err)
	}
	var sub database.Subscription
	if err := db.First(&sub, id).Error; err != nil || sub.MikanID != "51" {
		t.Fatalf("订阅未关联蜜柑: %+v %v", sub, err)
	}
}

func TestDownloadIndexUsesReceiptsBeforeNames(t *testing.T) {
	tasks := []core.DownloadTask{
		{Hash: "real-qb", TrackingTag: trackingTag("url:abc"), Name: "改名后的文件"},
		{Hash: "metadata-gid", ClientID: "metadata-gid", FollowedBy: []string{"child-gid"}, Name: "payload.torrent", Progress: 1, Status: "completed"},
		{Hash: "real-aria", ClientID: "child-gid", Name: "实际内容", Progress: 0.5, Status: "active"},
	}
	idx := newDownloadIndex(tasks, nil)
	qb, ok := idx.match(database.Episode{OriginKey: "url:abc", OriginalName: "发布标题"})
	if !ok || qb.Hash != "real-qb" {
		t.Fatalf("标签未匹配: %+v", qb)
	}
	child, ok := idx.match(database.Episode{TorrentHash: "metadata-gid"})
	if !ok || child.Hash != "real-aria" || child.IsComplete() {
		t.Fatalf("Aria2 元数据被当成已完成的视频: %+v", child)
	}
}

func TestLegacyBatchRecoversPreviouslyMovedFile(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", Season: 1, AnimeType: "tv"}
	savePipelineRow(t, db, &sub)
	root := t.TempDir()
	lib := t.TempDir()
	old := filepath.Join(lib, "old-E00.mkv")
	for path, body := range map[string]string{old: "first", filepath.Join(root, "S01E02.mkv"): "second"} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 0, Title: "Series [01-02]", OriginalName: "Series [01-02]", Status: "organized", FinalPath: old, TorrentHash: "hash"}
	savePipelineRow(t, db, &ep)
	down := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{tasks: []core.DownloadTask{{Hash: "hash", ContentPath: root, Progress: 1, Status: "completed"}}}}
	s := Scheduler{downloader: down, organizer: organizer.New("{title_cn}/S{season:02}E{ep:02}", "", "", lib, "", false, nil)}
	if err := s.repairLegacyEpisodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.pollOrganizer(context.Background())
	var episodes []database.Episode
	if err := db.Where("status = ?", "organized").Order("number").Find(&episodes).Error; err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 2 {
		t.Fatalf("旧合集未恢复: %+v", episodes)
	}
	for i, want := range []string{"first", "second"} {
		got, err := os.ReadFile(episodes[i].FinalPath)
		if err != nil || string(got) != want {
			t.Fatalf("第%d集: %q %v", i+1, got, err)
		}
	}
}

func TestSeedCleanupHonorsZeroHotRetention(t *testing.T) {
	db := pipelineDB(t)
	now := time.Now()
	savePipelineRow(t, db, &database.Episode{SubscriptionID: 1, Season: 1, Number: 1, Status: "organized", TorrentHash: "managed", FinalPath: "library.mkv", OrganizedAt: &now})
	seconds := database.Setting{Key: "SEED_CLEANUP_MIN_SEED_TIME", Value: "0s"}
	savePipelineRow(t, db, &seconds)
	down := &mockSchedDownloader{tasks: []core.DownloadTask{{Hash: "managed", Progress: 1, Status: "completed", Ratio: 3, SeedingTime: 1}}}
	cfg := &config.Config{}
	cfg.Scheduler.SeedCleanupMinSeedTime = 24 * time.Hour
	cfg.Scheduler.SeedCleanupMinRatio = 2
	s := Scheduler{cfg: cfg, downloader: down}
	s.pollSeedCleanup(context.Background())
	if len(down.deleted) != 1 {
		t.Fatal("热设置 0s 没有关闭做种时长条件")
	}
	if err := db.Model(&seconds).Update("value", "-1s").Error; err != nil {
		t.Fatal(err)
	}
	s.pollSeedCleanup(context.Background())
	if len(down.deleted) != 1 {
		t.Fatal("负值没有回退到已配置的做种时长")
	}
}

type delayedHistory struct {
	pipelineSource
	entered, release chan struct{}
}

func (d delayedHistory) FetchHistory(context.Context, string, core.Filter) ([]core.TorrentItem, error) {
	close(d.entered)
	<-d.release
	return d.history, nil
}

func TestCancelDuringHistoryPreventsDispatch(t *testing.T) {
	for _, mode := range []string{"disable", "delete"} {
		t.Run(mode, func(t *testing.T) {
			db := pipelineDB(t)
			sub := database.Subscription{TitleCN: "Series", Season: 1}
			savePipelineRow(t, db, &sub)
			d := delayedHistory{pipelineSource: pipelineSource{history: []core.TorrentItem{{Title: "Series - 01", URL: "test:item"}}}, entered: make(chan struct{}), release: make(chan struct{})}
			dl := &pipelineDownloader{}
			s := Scheduler{source: d, downloader: dl, cfg: &config.Config{}}
			finished := make(chan struct{})
			go func() { defer close(finished); s.supplementOne(context.Background(), sub) }()
			<-d.entered
			if mode == "disable" {
				if err := db.Model(&sub).Update("enabled", false).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := db.Delete(&sub).Error; err != nil {
					t.Fatal(err)
				}
			}
			close(d.release)
			<-finished
			var n int64
			db.Model(&database.Episode{}).Count(&n)
			if n != 0 || dl.calls.Load() != 0 {
				t.Fatalf("停用或删除后仍下发了种子: %d/%d", n, dl.calls.Load())
			}
		})
	}
}
func TestDeleteDuringSubmissionDoesNotWriteHistory(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", Season: 1}
	savePipelineRow(t, db, &sub)
	dl := &pipelineDownloader{started: make(chan struct{}), resume: make(chan struct{})}
	s := Scheduler{downloader: dl, cfg: &config.Config{}}
	done := make(chan bool)
	go func() {
		added, _ := s.enqueue(context.Background(), sub, core.TorrentItem{Title: "Series - 01", URL: "test:one"}, 1, 1)
		done <- added
	}()
	<-dl.started
	if err := db.Delete(&sub).Error; err != nil {
		t.Fatal(err)
	}
	close(dl.resume)
	added := <-done
	var n, history int64
	db.Model(&database.Episode{}).Count(&n)
	db.Model(&database.DownloadRecord{}).Count(&history)
	if added || n != 0 || history != 0 {
		t.Fatalf("取消后仍写入成功: %t / %d / %d", added, n, history)
	}
}

func TestLegacySeasonRepairPreservesConflictingOwner(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(fmt.Sprint(deleted), func(t *testing.T) {
			db := pipelineDB(t)
			sub := database.Subscription{TitleCN: "Series", Season: 2, AnimeType: "tv"}
			savePipelineRow(t, db, &sub)
			lib := t.TempDir()
			for i, season := range []int{1, 2} {
				target := filepath.Join(lib, "Series", fmt.Sprintf("S%02dE05.mkv", season))
				if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(fmt.Sprint(i)), 0600); err != nil {
					t.Fatal(err)
				}
				ep := database.Episode{SubscriptionID: sub.ID, Season: season, Number: 5, OriginalName: "Series - 05", Status: "organized", TorrentHash: fmt.Sprint(i), FinalPath: target}
				savePipelineRow(t, db, &ep)
				if i == 1 && deleted {
					if err := db.Delete(&ep).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			s := Scheduler{organizer: organizer.New("{title_cn}/S{season:02}E{ep:02}", "", "", lib, "", false, nil)}
			if err := s.repairLegacyEpisodes(context.Background()); err != nil {
				t.Fatal(err)
			}
			s.pollOrganizer(context.Background())
			var eps []database.Episode
			if err := db.Unscoped().Order("id").Find(&eps).Error; err != nil {
				t.Fatal(err)
			}
			for i, ep := range eps {
				body, err := os.ReadFile(ep.FinalPath)
				if ep.Season != i+1 || ep.Status != "organized" || err != nil || string(body) != fmt.Sprint(i) {
					t.Fatalf("已占用的季集被自动更名: %+v %q %v", ep, body, err)
				}
			}
		})
	}
}
func TestLegacyDownloadedAriaParentCannotOrganizeMetadata(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", AnimeType: "tv"}
	savePipelineRow(t, db, &sub)
	root := t.TempDir()
	meta := filepath.Join(root, "data.torrent")
	if err := os.WriteFile(meta, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, TorrentHash: "123456789abcdef0", Status: "downloaded"}
	savePipelineRow(t, db, &ep)
	parent := core.DownloadTask{Hash: ep.TorrentHash, Progress: 1, Status: "completed", ContentPath: meta, FollowedBy: []string{"child"}}
	child := core.DownloadTask{Hash: "childhash", ClientID: "child", Progress: 0.6, Status: "active"}
	dl := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{tasks: []core.DownloadTask{parent, child}}}
	s := Scheduler{downloader: dl, organizer: organizer.New("{title_cn}/S{season:02}E{ep:02}", "", "", t.TempDir(), "", false, nil)}
	s.pollOrganizer(context.Background())
	s.pollDownloads(context.Background())
	if err := db.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(meta); err != nil || ep.TorrentHash != "childhash" || ep.Status != "downloading" || ep.FinalPath != "" {
		t.Fatalf("元数据被整理或未迁至子任务: %+v %v", ep, err)
	}
}
