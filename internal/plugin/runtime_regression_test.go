package plugin

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestFailedProviderReplacementRestoresActiveService(t *testing.T) {
	root := core.NewContext(context.Background())
	root.SetService("resource", "host")
	first := NewPluginContext("first", nil, root)
	first.RegisterService("resource", "working")
	replacement := NewPluginContext("replacement", nil, root)
	replacement.RegisterService("resource", "starting")
	replacement.Dispose()
	if v, ok := root.GetService("resource"); !ok || v != "working" {
		t.Errorf("上一个服务未恢复: %v", v)
	}
	first.Dispose()
	if v, ok := root.GetService("resource"); !ok || v != "host" {
		t.Errorf("宿主服务未恢复: %v", v)
	}
}

func TestDisposeDrainsSubscribedHandlersBeforeReleasingResources(t *testing.T) {
	bus := newTestBus()
	c := NewPluginContext("observer", bus, nil)
	entered, finish, released := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c.OnDispose(func() { close(released) })
	c.Subscribe("work", func(core.Event) {
		calls.Add(1)
		close(entered)
		<-finish
	})
	go bus.Publish(core.Event{Type: "work"})
	<-entered
	done := make(chan struct{})
	go func() { c.Dispose(); close(done) }()
	<-c.Done()
	select {
	case <-released:
		t.Error("仍有运行中的订阅回调时资源已经释放")
	case <-time.After(40 * time.Millisecond):
	}
	bus.Publish(core.Event{Type: "work"})
	close(finish)
	<-done
	if calls.Load() != 1 {
		t.Fatal("取消订阅后仍分发了事件")
	}
	select {
	case <-released:
	default:
		t.Fatal("卸载未释放资源")
	}
}

func TestLifecycleReceiptDeletionAndRestore(t *testing.T) {
	db := setupTestDB(t)
	m := NewManager(newTestBus())
	defer m.StopAll()
	p := PluginInfo{Name: "钩子", Type: "webhook", URL: "https://example.org/hook"}
	secret := "testing"
	info, err := m.SaveCustomPluginInfo(p, &secret)
	if err != nil || info.ID == "" || info.Secret != "" || info.Status != StateActive || !info.Enabled {
		t.Fatalf("保存响应错误 %+v %v", info, err)
	}
	p.ID = info.ID
	if err := m.Disable(p.ID); err != nil {
		t.Fatal(err)
	}
	info, err = m.SaveCustomPluginInfo(p, nil)
	if err != nil || info.Enabled || info.Status != StateInactive {
		t.Fatalf("编辑响应失真 %+v %v", info, err)
	}
	if err := m.DeletePlugin(p.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.DeletePlugin(p.ID); !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("不存在的插件错误 %v", err)
	}
	info, err = m.SaveCustomPluginInfo(p, nil)
	if err != nil || !info.Enabled || info.Status != StateActive {
		t.Fatal("重装继承旧开关")
	}
	if _, err := m.SaveCustomPluginInfo(PluginInfo{ID: "unsupported", Name: "脚本", Type: "script"}, nil); !errors.Is(err, ErrInvalidPluginManifest) {
		t.Fatal("接受了无运行器的类型")
	}
	failure := errors.New("restore failed")
	old := m.GetPluginContext(p.ID)
	if err := m.RestoreConfiguration(func() error { return failure }); !errors.Is(err, failure) || m.GetPluginContext(p.ID) != old {
		t.Fatal("失败的恢复改变了运行上下文")
	}
	if err := m.RestoreConfiguration(func() error { storePluginConfig(t, db, "custom_plugins", "["); return nil }); !errors.Is(err, ErrReloadAfterRestore) || m.GetPluginContext(p.ID) != nil || old.Err() == nil {
		t.Fatal("提交后加载失败，旧授权仍可使用或未报告错误")
	}
	m.SetCoreService("late", "service")
	if m.IsPluginActive(p.ID) {
		t.Fatal("普通服务变更重新启动了过期的配置")
	}
	storePluginConfig(t, db, "custom_plugins", "[]")
	if err := m.Load(); err != nil || m.GetPluginContext(p.ID) != nil {
		t.Fatal("恢复后未卸载旧实例")
	}
	if status, _ := m.GetPluginStatus(p.ID); status != StateInactive {
		t.Fatal("恢复已删除的插件仍报告活动态")
	}
}

type lifecycleObserver struct {
	dependencyFixture
	manager              *Manager
	initState, stopState PluginState
	newScope, oldScope   PluginContext
	hold                 chan struct{}
}

func (p *lifecycleObserver) Init(_ core.EventBus, ctx core.Context) error {
	p.starts++
	p.initState, _ = p.manager.GetPluginStatus(p.id)
	p.newScope = p.manager.GetPluginContext(p.id)
	if p.hold != nil && p.starts == 1 {
		ctx.(PluginContext).Go(func(context.Context) { <-p.hold })
	}
	return nil
}
func (p *lifecycleObserver) Stop(core.EventBus) error {
	p.stopState, _ = p.manager.GetPluginStatus(p.id)
	p.oldScope = p.manager.GetPluginContext(p.id)
	return nil
}

func TestLifecycleReadsDuringCallbacksAndOutstandingGeneration(t *testing.T) {
	setupTestDB(t)
	m := NewManager(nil)
	finish := make(chan struct{})
	p := &lifecycleObserver{dependencyFixture: dependencyFixture{id: "readable"}, manager: m, hold: finish}
	m.builtInPlugins = []BuiltInPlugin{p}
	defer m.StopAll()
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if p.initState != StateLoading || p.newScope != nil {
		t.Error("初始化中意外暴露了可用实例")
	}
	if err := m.Disable(p.id); err != nil {
		t.Fatal(err)
	}
	if p.stopState != StateDisposing || p.oldScope != nil {
		t.Error("停用中意外暴露了可用实例")
	}
	if err := m.Enable(p.id); err != nil {
		t.Fatal(err)
	}
	if state, _ := m.GetPluginStatus(p.id); state != StateDisposing || p.starts != 1 {
		t.Fatal("旧实例未退出时已启动新实例")
	}
	close(finish)
	deadline := time.Now().Add(time.Second)
	for !m.IsPluginActive(p.id) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !m.IsPluginActive(p.id) || p.starts != 2 {
		t.Fatal("旧实例退出后未恢复最新启用意愿")
	}
}

func TestReloadUsesLatestRegistryBusIncludingLegacySubscriber(t *testing.T) {
	setupTestDB(t)
	oldBus, newBus := newTestBus(), newTestBus()
	base := core.NewContext(context.Background())
	m := NewManager(oldBus, base)
	p := &eventDependency{dependencyFixture: dependencyFixture{id: "legacy-subscriber"}}
	m.builtInPlugins = []BuiltInPlugin{p}
	defer m.StopAll()
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	base.SetService(core.ServiceEventBus, newBus)
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	newBus.Publish(core.Event{Type: "dependency-event"})
	if p.deliveries != 1 || m.GetBus() != newBus || p.stopBus != oldBus {
		t.Fatal("没有在注册表的新总线上接收事件")
	}
	oldBus.Publish(core.Event{Type: "dependency-event"})
	if p.deliveries != 1 {
		t.Fatal("旧总线仍在投递")
	}
}

func TestSaveCustomPluginRejectsDotSegments(t *testing.T) {
	setupTestDB(t)
	m := NewManager(nil)
	defer m.StopAll()
	for _, id := range []string{".", ".."} {
		if _, err := m.SaveCustomPluginInfo(PluginInfo{ID: id, Name: id, URL: "https://example.org/hook"}, nil); !errors.Is(err, ErrInvalidPluginManifest) {
			t.Errorf("接受不可寻址 ID %q: %v", id, err)
		}
	}
}

func TestRestoreRetiresOldScopesForUnchangedManifest(t *testing.T) {
	setupTestDB(t)
	m := NewManager(nil)
	defer m.StopAll()
	p, err := m.SaveCustomPluginInfo(PluginInfo{ID: "same", Name: "Same", URL: "https://example.org/actions"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	old := m.GetPluginContext(p.ID)
	if old == nil {
		t.Fatal("缺少恢复前的实例")
	}
	if err := m.RestoreConfiguration(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	next := m.GetPluginContext(p.ID)
	if old.Err() == nil || next == nil || old == next {
		t.Fatal("恢复后仍允许之前的在途请求使用旧授权")
	}
}

type flakyProvider struct {
	dependencyFixture
	fail bool
}

func (p *flakyProvider) Init(_ core.EventBus, ctx core.Context) error {
	ctx.(PluginContext).RegisterService("test-resource", "replacement")
	if p.fail {
		return errors.New("not ready")
	}
	return nil
}

func TestFailingProviderDoesNotInterruptHealthyConsumers(t *testing.T) {
	setupTestDB(t)
	root := core.NewContext(context.Background())
	root.SetService("test-resource", "working")
	m := NewManager(nil, root)
	consumer := &dependencyFixture{id: "consumer", needs: []core.ServiceType{"test-resource"}}
	provider := &flakyProvider{dependencyFixture: dependencyFixture{id: "provider"}, fail: true}
	m.builtInPlugins = []BuiltInPlugin{consumer, provider}
	defer m.StopAll()
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if !m.IsPluginActive(consumer.id) || consumer.starts != 1 || consumer.stops != 0 || root.MustGetService("test-resource") != "working" {
		t.Fatal("失败的替代提供者破坏了健康的消费方")
	}
	provider.fail = false
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if state, _ := m.GetPluginStatus(provider.id); state != StateActive || consumer.starts != 2 || consumer.stops != 1 || root.MustGetService("test-resource") != "replacement" {
		t.Fatal("提供者恢复后消费方没有重新绑定")
	}
}

type replayingBus struct{ *testBus }

func (b *replayingBus) Subscribe(event string, h core.EventHandler) core.SubscriptionID {
	id := b.testBus.Subscribe(event, h)
	h(core.Event{Type: event})
	return id
}
func TestSynchronousReplayMayEnterAndDisposeScope(t *testing.T) {
	bus := &replayingBus{newTestBus()}
	c := NewPluginContext("replay", bus, nil)
	done := make(chan struct{})
	go func() {
		c.Subscribe("state", func(core.Event) { c.OnDispose(func() {}) })
		c.Dispose()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("同步回放在订阅过程中死锁")
	}
}
