package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

type stubOrganizerHookProvider struct {
	hookMgr *core.WaterfallHookManager
}

func (s *stubOrganizerHookProvider) GetHookManager() *core.WaterfallHookManager {
	return s.hookMgr
}

func TestGeekNamingPlugin_DynamicServiceOrganizerLifecycle(t *testing.T) {
	setupTestDB(t)
	bus := newTestBus()
	root := core.NewContext(context.Background())
	hookMgr := core.NewWaterfallHookManager()
	root.SetService(core.ServiceOrganizer, &stubOrganizerHookProvider{hookMgr: hookMgr})

	mgr := NewManager(bus, root)
	defer mgr.StopAll()
	if err := mgr.Load(); err != nil {
		t.Fatalf("加载插件失败: %v", err)
	}
	if !mgr.IsPluginActive("standard-naming") {
		t.Fatal("standard-naming 默认应处于运行中状态")
	}

	raw := "Anime : Name / Ep 01.mkv"
	got, cancelled, reason := hookMgr.ExecuteNamingHooks(context.Background(), core.NamingHookInput{RenderedPath: raw})
	if cancelled || reason != "" {
		t.Fatalf("执行命名钩子失败: reason=%q cancelled=%v", reason, cancelled)
	}
	if got == raw {
		t.Fatalf("启用 standard-naming 后路径未被清洗: %q", got)
	}

	if err := mgr.Disable("standard-naming"); err != nil {
		t.Fatalf("停用 standard-naming 失败: %v", err)
	}
	gotAfterStop, cancelled, reason := hookMgr.ExecuteNamingHooks(context.Background(), core.NamingHookInput{RenderedPath: raw})
	if cancelled || reason != "" {
		t.Fatalf("停用后执行命名钩子异常: reason=%q cancelled=%v", reason, cancelled)
	}
	if gotAfterStop != raw {
		t.Fatalf("停用 standard-naming 后钩子仍修改了路径: got=%q want=%q", gotAfterStop, raw)
	}

	if err := mgr.Enable("standard-naming"); err != nil {
		t.Fatalf("重新启用 standard-naming 失败: %v", err)
	}
	gotReenabled, cancelled, reason := hookMgr.ExecuteNamingHooks(context.Background(), core.NamingHookInput{RenderedPath: raw})
	if cancelled || reason != "" || gotReenabled == raw {
		t.Fatalf("重新启用 standard-naming 后钩子未生效: got=%q cancelled=%v reason=%q", gotReenabled, cancelled, reason)
	}
}

type fakeNotifyController struct {
	enabled      bool
	enableCalls  int
	disableCalls int
	reloadCalls  int
}

func (f *fakeNotifyController) SetEnabled(enabled bool) {
	f.enabled = enabled
	if enabled {
		f.enableCalls++
	} else {
		f.disableCalls++
	}
}

func (f *fakeNotifyController) IsEnabled() bool {
	return f.enabled
}

func (f *fakeNotifyController) Reload() {
	f.reloadCalls++
}

func TestExtendedNotifiersPlugin_DiscoversServiceNotifier(t *testing.T) {
	bus := newTestBus()
	root := core.NewContext(context.Background())
	ctrl := &fakeNotifyController{}
	root.SetService(core.ServiceNotifier, ctrl)

	p := &ExtendedNotifiersPlugin{}
	pctx := NewPluginContext("extended-notifiers", bus, root)
	if err := p.Init(bus, pctx); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	if !ctrl.enabled || ctrl.enableCalls != 1 || ctrl.reloadCalls != 1 {
		t.Fatalf("未通过 ServiceNotifier 正确激活控制器: %+v", ctrl)
	}

	if err := p.Stop(bus); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}
	if ctrl.enabled || ctrl.disableCalls != 1 {
		t.Fatalf("Stop 应仅停用控制器一次: %+v", ctrl)
	}
}

func TestMetadataMappingPlugin_DedupAndResolveTMDBConfig(t *testing.T) {
	db := setupTestDB(t)

	p := &MetadataMappingPlugin{}
	if !p.acquireInFlight(101) {
		t.Fatal("首次获取在途锁应成功")
	}
	if p.acquireInFlight(101) {
		t.Fatal("同一订阅并发在途任务应被去重")
	}
	if !p.acquireInFlight(102) {
		t.Fatal("不同订阅不应互相阻塞")
	}
	p.releaseInFlight(101)
	p.releaseInFlight(102)
	if !p.acquireInFlight(101) {
		t.Fatal("释放后应允许再次获取在途锁")
	}
	p.releaseInFlight(101)

	t.Setenv("TMDB_API_KEY", "env-key")
	t.Setenv("TMDB_LANGUAGE", "ja-JP")
	t.Setenv("TMDB_MIRROR_DOMAINS", "https://env.mirror.example")

	k, lang, mirrors := resolveTMDBConfig(db)
	if k != "env-key" || lang != "ja-JP" || len(mirrors) != 1 || mirrors[0] != "https://env.mirror.example" {
		t.Fatalf("环境变量回退配置错误: key=%q lang=%q mirrors=%v", k, lang, mirrors)
	}

	if err := db.Save(&database.Setting{Key: "TMDB_API_KEY", Value: "db-key"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Save(&database.Setting{Key: "TMDB_LANGUAGE", Value: "zh-TW"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Save(&database.Setting{Key: "TMDB_MIRROR_DOMAINS", Value: "https://db1.example, https://db2.example"}).Error; err != nil {
		t.Fatal(err)
	}

	k, lang, mirrors = resolveTMDBConfig(db)
	if k != "db-key" || lang != "zh-TW" || len(mirrors) != 2 || mirrors[0] != "https://db1.example" || mirrors[1] != "https://db2.example" {
		t.Fatalf("数据库覆盖配置错误: key=%q lang=%q mirrors=%v", k, lang, mirrors)
	}
}

func TestMetadataMappingPlugin_MovieSeriesIDPreservesExistingTMDB(t *testing.T) {
	db := setupTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/movie/372058" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":372058,"title":"你的名字。","original_title":"君の名は。","release_date":"2016-08-26","belongs_to_collection":{"id":10,"name":"新海诚作品集"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if err := db.Save(&database.Setting{Key: "TMDB_API_KEY", Value: "test-key"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Save(&database.Setting{Key: "TMDB_MIRROR_DOMAINS", Value: srv.URL}).Error; err != nil {
		t.Fatal(err)
	}

	sub := database.Subscription{
		TitleCN:          "你的名字。",
		MetadataProvider: "BGM.tv",
		MetadataID:       "160447",
		TMDBID:           "movie/372058",
		IMDBID:           "tt5311514",
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}

	p := &MetadataMappingPlugin{}
	p.enabled.Store(true)
	pctx := NewPluginContext("metadata-mapping", newTestBus(), nil)
	p.mapOne(t.Context(), sub.ID, pctx)

	var updated database.Subscription
	if err := db.First(&updated, sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.TMDBID != "movie/372058" {
		t.Fatalf("原有 TMDBID 被意外改写: %q", updated.TMDBID)
	}
	if updated.SeriesID != "新海诚作品集" {
		t.Fatalf("movie/ 前缀剧场版未能正确补全 SeriesID: %q", updated.SeriesID)
	}
}

func TestNFOScraperPlugin_ZeroValuePosterMapSafe(t *testing.T) {
	bus := newTestBus()
	p := &NFOScraperPlugin{}
	if err := p.Init(bus, nil); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	defer p.Stop(bus)

	// SSRF 校验之前就会检查 posterInFlight，验证零值实例不会因 nil map 崩溃
	p.downloadPoster(t.Context(), t.TempDir(), "http://127.0.0.1:1/poster.jpg", nil)
}

func TestOfficialPlugins_ConsistentMetadata(t *testing.T) {
	setupTestDB(t)
	mgr := NewManager(newTestBus())
	list := mgr.GetPluginList()

	expectedIDs := map[string]bool{
		"metadata-mapping":   false,
		"standard-naming":    false,
		"yuc_schedule":       false,
		"data-migrate":       false,
		"extended-notifiers": false,
		"nfo_scraper":        false,
		"cloud-115":          false,
	}

	for _, info := range list {
		if _, ok := expectedIDs[info.ID]; !ok {
			continue
		}
		expectedIDs[info.ID] = true
		if info.Origin != OriginOfficial || !info.IsBuiltIn {
			t.Errorf("插件 %s 来源异常: origin=%q builtin=%v", info.ID, info.Origin, info.IsBuiltIn)
		}
		if info.Author != "xiaoyueRX" {
			t.Errorf("插件 %s Author 应统一为 xiaoyueRX，实际: %q", info.ID, info.Author)
		}
		if strings.Contains(info.Description, "後") {
			t.Errorf("插件 %s 描述包含繁体错字: %q", info.ID, info.Description)
		}
		if info.ID == "standard-naming" && len(info.Events) != 0 {
			t.Errorf("standard-naming 使用命名钩子，Events 应为空，实际: %v", info.Events)
		}
		if info.ID == "cloud-115" && info.Panel == nil {
			t.Errorf("cloud-115 应包含声明式面板")
		}
	}

	for id, seen := range expectedIDs {
		if !seen {
			t.Errorf("缺少官方插件: %s", id)
		}
	}
}
