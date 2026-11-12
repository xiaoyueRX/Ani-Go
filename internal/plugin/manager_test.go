package plugin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

type testSub struct {
	id core.SubscriptionID
	fn core.EventHandler
}

type testBus struct {
	mu       sync.RWMutex
	handlers map[string][]testSub
	subCount uint64
}

func newTestBus() *testBus {
	return &testBus{
		handlers: make(map[string][]testSub),
	}
}

func (b *testBus) Publish(event core.Event) {
	b.mu.RLock()
	subs := make([]testSub, len(b.handlers[event.Type]))
	copy(subs, b.handlers[event.Type])
	b.mu.RUnlock()

	for _, s := range subs {
		s.fn(event)
	}
}

func (b *testBus) Subscribe(eventType string, handler core.EventHandler) core.SubscriptionID {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subCount++
	id := core.SubscriptionID(b.subCount)
	b.handlers[eventType] = append(b.handlers[eventType], testSub{id: id, fn: handler})
	return id
}

func (b *testBus) Unsubscribe(eventType string, id core.SubscriptionID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.handlers[eventType]
	for i, s := range list {
		if s.id == id {
			b.handlers[eventType] = append(list[:i], list[i+1:]...)
			return
		}
	}
}

// 恐慌测试插件
type panicPlugin struct{}

func (p *panicPlugin) GetInfo() PluginInfo {
	return PluginInfo{
		ID:        "panic_plugin",
		Name:      "Panic Test Plugin",
		Version:   "1.0.0",
		IsBuiltIn: true,
	}
}

func (p *panicPlugin) Init(bus core.EventBus, ctx core.Context) error {
	panic("crash during plugin init!")
}

func (p *panicPlugin) Stop(bus core.EventBus) error {
	panic("crash during plugin stop!")
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}
	database.DB = db
	_ = db.AutoMigrate(&database.Setting{}, &database.Subscription{})
	return db
}

func TestPluginManager_LifecycleAndPanicIsolation(t *testing.T) {
	setupTestDB(t)

	bus := newTestBus()
	mgr := NewManager(bus)

	// 注册会引发 panic 的插件测试沙箱隔离
	mgr.builtInPlugins = append(mgr.builtInPlugins, &panicPlugin{})

	// Load 不应崩溃（panic 被 defer recover 捕获）
	mgr.Load()

	plugins := mgr.List()
	if len(plugins) == 0 {
		t.Fatalf("插件列表不应为空")
	}

	foundNFO := false
	for _, p := range plugins {
		if p.ID == "nfo_scraper" {
			foundNFO = true
			if p.Enabled {
				t.Errorf("nfo_scraper 默认状态应为关闭 (Opt-in)")
			}
		}
	}
	if !foundNFO {
		t.Errorf("未能检索到内建 nfo_scraper 插件")
	}

	// 测试 Enable / Disable
	if err := mgr.Enable("nfo_scraper"); err != nil {
		t.Fatalf("启用 nfo_scraper 失败: %v", err)
	}
	if !mgr.IsPluginEnabled("nfo_scraper") {
		t.Errorf("nfo_scraper 应处于已启用状态")
	}

	if err := mgr.Disable("nfo_scraper"); err != nil {
		t.Fatalf("停用 nfo_scraper 失败: %v", err)
	}
	if mgr.IsPluginEnabled("nfo_scraper") {
		t.Errorf("nfo_scraper 应处于已停用状态")
	}

	// 测试 StopAll
	mgr.StopAll()
}

func TestPluginManager_WebhookCustomPluginAndSSRF(t *testing.T) {
	setupTestDB(t)

	type capturedReq struct {
		ua     string
		secret string
		ct     string
		body   []byte
	}
	reqChan := make(chan capturedReq, 10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		reqChan <- capturedReq{
			ua:     r.Header.Get("User-Agent"),
			secret: r.Header.Get("X-AniGo-Secret"),
			ct:     r.Header.Get("Content-Type"),
			body:   body,
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	bus := newTestBus()
	mgr := NewManager(bus)
	mgr.allowPrivateWebhookForTest = true // 允许连接测试服务器

	// 1. 测试禁止 SSRF 地址（在常规校验下拦截私网 IP / localhost）
	_, err := validateExternalHTTPURL("http://127.0.0.1:8080/hook", false)
	if err == nil {
		t.Errorf("常规模式下应严格阻断 127.0.0.1 Webhook 地址")
	}
	_, err = validateExternalHTTPURL("http://localhost:8080/hook", false)
	if err == nil {
		t.Errorf("常规模式下应严格阻断 localhost Webhook 地址")
	}
	_, err = validateExternalHTTPURL("http://169.254.169.254/meta", false)
	if err == nil {
		t.Errorf("常规模式下应严格阻断云元数据地址")
	}

	// 2. 注册合法自定义 Webhook 插件
	custom := PluginInfo{
		ID:        "custom-webhook-1",
		Name:      "测试 Webhook",
		Type:      "webhook",
		URL:       server.URL + "/webhook",
		Secret:    "secret-12345",
		Events:    []string{core.EventDownloadCompleted},
		IsBuiltIn: false,
	}

	if err := mgr.AddOrUpdatePlugin(custom); err != nil {
		t.Fatalf("添加自定义 Webhook 插件失败: %v", err)
	}

	// 3. 发布事件并验证 Webhook 请求到达
	bus.Publish(core.Event{
		Type: core.EventDownloadCompleted,
		Payload: map[string]interface{}{
			"hash": "abc12345",
			"name": "Test Anime S01E01",
		},
		Time: time.Now(),
	})

	var firstReq capturedReq
	select {
	case firstReq = <-reqChan:
	case <-time.After(2 * time.Second):
		t.Fatalf("Webhook 目标服务器在超时时间内未收到任何请求")
	}

	if firstReq.ct != "application/json" {
		t.Errorf("Content-Type 头不符合规范: %s", firstReq.ct)
	}
	if firstReq.ua != "Ani-Go-Webhook/custom-webhook-1" {
		t.Errorf("User-Agent 头不符合规范: %s", firstReq.ua)
	}
	if firstReq.secret != "secret-12345" {
		t.Errorf("X-AniGo-Secret 头不匹配: %s", firstReq.secret)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(firstReq.body, &payload); err != nil {
		t.Fatalf("Webhook 负载不是有效 JSON: %v, body: %s", err, string(firstReq.body))
	}
	if payload["event"] != core.EventDownloadCompleted {
		t.Errorf("Webhook 事件类型不匹配: %v", payload["event"])
	}

	// 4. 删除插件后不再触发
	if err := mgr.DeletePlugin("custom-webhook-1"); err != nil {
		t.Fatalf("删除自定义插件失败: %v", err)
	}

	bus.Publish(core.Event{
		Type: core.EventDownloadCompleted,
		Payload: map[string]interface{}{
			"hash": "def67890",
		},
		Time: time.Now(),
	})

	select {
	case unexpected := <-reqChan:
		t.Errorf("删除自定义插件后仍触发了 Webhook 请求: %+v", unexpected)
	case <-time.After(150 * time.Millisecond):
		// 预期正常超时未收到
	}

	mgr.StopAll()
}

func TestPluginManager_ConcurrencyAndThreadSafety(t *testing.T) {
	setupTestDB(t)

	bus := newTestBus()
	mgr := NewManager(bus)
	mgr.Load()

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 并发读取 IsPluginEnabled
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				_ = mgr.IsPluginEnabled("metadata-mapping")
				_ = mgr.IsPluginEnabled("yuc_schedule")
				_ = mgr.List()
			}
		}()
	}

	// 并发写 TogglePlugin
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state := true
			for ctx.Err() == nil {
				_ = mgr.TogglePlugin("yuc_schedule", state)
				state = !state
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
	mgr.StopAll()
}

func TestBuiltInPlugins_AllComponents(t *testing.T) {
	setupTestDB(t)
	bus := newTestBus()

	// 1. DataMigratePlugin
	dm := &DataMigratePlugin{}
	if err := dm.Init(bus, nil); err != nil {
		t.Fatalf("DataMigratePlugin Init 失败: %v", err)
	}
	bus.Publish(core.Event{
		Type: "data.migrate",
		Payload: map[string]interface{}{
			"subscriptions": 10,
			"episodes":      120,
			"downloads":     float64(50),
		},
		Time: time.Now(),
	})
	stats := dm.GetStats()
	if stats["migrated_subs"] != 10 || stats["migrated_episodes"] != 120 || stats["migrated_downloads"] != 50 {
		t.Errorf("DataMigratePlugin 统计数据不符: %+v", stats)
	}
	if err := dm.Stop(bus); err != nil {
		t.Fatalf("DataMigratePlugin Stop 失败: %v", err)
	}

	// 2. GeekNamingPlugin & sanitizePathForMediaServer
	gn := DefaultGeekNamingPlugin
	if err := gn.Init(bus, nil); err != nil {
		t.Fatalf("GeekNamingPlugin Init 失败: %v", err)
	}
	pathTest := "Anime Name : Special <Season> 1 / Episode 01.mkv"
	sanitized := sanitizePathForMediaServer(pathTest)
	if sanitized == "" || sanitized == pathTest {
		t.Errorf("路径清洗未生效: %s -> %s", pathTest, sanitized)
	}
	if err := gn.Stop(bus); err != nil {
		t.Fatalf("GeekNamingPlugin Stop 失败: %v", err)
	}

	// 3. ExtendedNotifiersPlugin
	extNotif := DefaultExtendedNotifiersPlugin
	if err := extNotif.Init(bus, nil); err != nil {
		t.Fatalf("ExtendedNotifiersPlugin Init 失败: %v", err)
	}
	if err := extNotif.Stop(bus); err != nil {
		t.Fatalf("ExtendedNotifiersPlugin Stop 失败: %v", err)
	}

	// 4. MetadataMappingPlugin
	mm := &MetadataMappingPlugin{}
	if err := mm.Init(bus, nil); err != nil {
		t.Fatalf("MetadataMappingPlugin Init 失败: %v", err)
	}
	if err := mm.Init(bus, nil); err != nil {
		t.Fatalf("MetadataMappingPlugin 重复 Init 失败: %v", err)
	}
	if err := mm.Stop(bus); err != nil {
		t.Fatalf("MetadataMappingPlugin Stop 失败: %v", err)
	}
	if err := mm.Stop(bus); err != nil {
		t.Fatalf("MetadataMappingPlugin 重复 Stop 失败: %v", err)
	}

	// 5. YucSchedulePlugin
	yuc := &YucSchedulePlugin{}
	if err := yuc.Init(bus, nil); err != nil {
		t.Fatalf("YucSchedulePlugin Init 失败: %v", err)
	}
	if err := yuc.Stop(bus); err != nil {
		t.Fatalf("YucSchedulePlugin Stop 失败: %v", err)
	}
}
