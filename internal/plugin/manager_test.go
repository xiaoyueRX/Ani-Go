package plugin

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

type testBus struct {
	handlers map[string][]core.EventHandler
	subCount int
}

func newTestBus() *testBus {
	return &testBus{
		handlers: make(map[string][]core.EventHandler),
	}
}

func (b *testBus) Publish(event core.Event) {
	if list, ok := b.handlers[event.Type]; ok {
		for _, h := range list {
			h(event)
		}
	}
}

func (b *testBus) Subscribe(eventType string, handler core.EventHandler) core.SubscriptionID {
	b.subCount++
	b.handlers[eventType] = append(b.handlers[eventType], handler)
	return core.SubscriptionID(b.subCount)
}

func (b *testBus) Unsubscribe(eventType string, id core.SubscriptionID) {
	// 简化测试实现
	delete(b.handlers, eventType)
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

func TestPluginManager_LifecycleAndPanicIsolation(t *testing.T) {
	// 初始化 SQLite 内存测试库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}
	database.DB = db
	_ = db.AutoMigrate(&database.Setting{})

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
}
