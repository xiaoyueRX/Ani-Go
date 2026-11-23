package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type dependencyFixture struct {
	id            string
	needs         []core.ServiceType
	provides      core.ServiceType
	log           *[]string
	starts, stops int
}

func (p *dependencyFixture) GetInfo() PluginInfo              { return PluginInfo{ID: p.id, Name: p.id} }
func (p *dependencyFixture) Dependencies() []core.ServiceType { return p.needs }
func (p *dependencyFixture) Init(_ core.EventBus, ctx core.Context) error {
	p.starts++
	if p.log != nil {
		*p.log = append(*p.log, "+"+p.id)
	}
	if p.provides != "" {
		ctx.(PluginContext).RegisterService(p.provides, p.starts)
	}
	return nil
}
func (p *dependencyFixture) Stop(core.EventBus) error {
	p.stops++
	if p.log != nil {
		*p.log = append(*p.log, "-"+p.id)
	}
	return nil
}

func TestDependencyChainSuspendsAndRecoversInLoadOrder(t *testing.T) {
	setupTestDB(t)
	var order []string
	client := &dependencyFixture{id: "client", needs: []core.ServiceType{"second"}, log: &order}
	middle := &dependencyFixture{id: "middle", needs: []core.ServiceType{"first"}, provides: "second", log: &order}
	provider := &dependencyFixture{id: "provider", provides: "first", log: &order}
	m := NewManager(newTestBus())
	m.builtInPlugins = []BuiltInPlugin{client, middle, provider}
	defer m.StopAll()
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"+provider", "+middle", "+client"}) {
		t.Fatalf("启动顺序 %v", order)
	}
	if err := m.Disable(provider.id); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order[3:], []string{"-client", "-middle", "-provider"}) {
		t.Fatalf("依赖链停用顺序 %v", order)
	}
	for _, p := range m.List() {
		if p.ID == provider.id {
			continue
		}
		var fields map[string]any
		raw, _ := json.Marshal(p)
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if !p.Enabled || fields["status"] != "suspended" || !reflect.DeepEqual(fields["dependencies"], fields["missing_dependencies"]) {
			t.Fatalf("缺少依赖状态 %s", raw)
		}
	}
	if err := m.Enable(provider.id); err != nil {
		t.Fatal(err)
	}
	if client.starts != 2 || middle.starts != 2 {
		t.Fatalf("恢复启动次数 client=%d middle=%d", client.starts, middle.starts)
	}
	order = nil
	m.StopAll()
	m.StopAll()
	if !reflect.DeepEqual(order, []string{"-client", "-middle", "-provider"}) {
		t.Fatalf("退出释放顺序 %v", order)
	}
	if err := m.Enable(provider.id); err == nil {
		t.Fatal("退出后重新启动了插件")
	}
}
func TestCoreDependencyReplacementAndOwnership(t *testing.T) {
	setupTestDB(t)
	m := NewManager(newTestBus())
	client := &dependencyFixture{id: "client", needs: []core.ServiceType{"clock"}}
	m.builtInPlugins = []BuiltInPlugin{client}
	defer m.StopAll()
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	for _, s := range []any{1, 2, nil, 3} {
		m.SetCoreService("clock", s)
	}
	if client.starts != 3 || client.stops != 2 {
		t.Fatalf("替换后启动 %d 次停用 %d 次", client.starts, client.stops)
	}
	first := NewPluginContext("one", m.bus, m.coreCtx)
	second := NewPluginContext("two", m.bus, m.coreCtx)
	first.RegisterService("shared", []string{"same"})
	second.RegisterService("shared", []string{"same"})
	first.Dispose()
	if _, ok := m.coreCtx.GetService("shared"); !ok {
		t.Fatal("旧提供者清除了替换者")
	}
	second.Dispose()
	if svc, ok := m.coreCtx.GetService("shared"); ok && svc != nil {
		t.Fatal("未清理拥有的服务")
	}
}
func TestMissingServiceCycleAndLatePublication(t *testing.T) {
	setupTestDB(t)
	m := NewManager(newTestBus())
	a := &dependencyFixture{id: "a", needs: []core.ServiceType{"b"}, provides: "a"}
	b := &dependencyFixture{id: "b", needs: []core.ServiceType{"a"}, provides: "b"}
	pub := &dependencyFixture{id: "pub"}
	late := &dependencyFixture{id: "late", needs: []core.ServiceType{"late-service"}}
	m.builtInPlugins = []BuiltInPlugin{a, b, pub, late}
	defer m.StopAll()
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if a.starts+b.starts != 0 {
		t.Fatal("环形依赖错误启动")
	}
	ctx := m.GetPluginContext("pub")
	ctx.RegisterService("late-service", true)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if state, _ := m.GetPluginStatus("late"); state == "active" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("已挂起的插件未响应运行时服务注册")
}

type stopAfterCancellation struct {
	dependencyFixture
	jobDone chan struct{}
	orderly bool
}

func (p *stopAfterCancellation) Init(_ core.EventBus, ctx core.Context) error {
	p.jobDone = make(chan struct{})
	ctx.(PluginContext).Go(func(c context.Context) { <-c.Done(); close(p.jobDone) })
	return nil
}
func (p *stopAfterCancellation) Stop(core.EventBus) error {
	select {
	case <-p.jobDone:
		p.orderly = true
	case <-time.After(300 * time.Millisecond):
	}
	return nil
}

func TestStopCancelsScopeBeforeCallingPluginAndRemovedDependenciesRebind(t *testing.T) {
	setupTestDB(t)
	m := NewManager(newTestBus())
	p := &stopAfterCancellation{dependencyFixture: dependencyFixture{id: "wait", needs: []core.ServiceType{"one", "two"}}}
	m.builtInPlugins = []BuiltInPlugin{p}
	defer m.StopAll()
	m.SetCoreService("one", 1)
	m.SetCoreService("two", 2)
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	original := m.GetPluginContext(p.id)
	p.needs = []core.ServiceType{"one"}
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if m.GetPluginContext(p.id) == original || !p.orderly {
		t.Fatal("移除声明后未重新绑定，或停用回调在取消之前执行")
	}
	p.orderly = false
	m.StopAll()
	if !p.orderly {
		t.Fatal("关闭时等待工作者的插件未先收到取消信号")
	}
}

type eventDependency struct {
	dependencyFixture
	deliveries int
	stopBus    core.EventBus
}

func (p *eventDependency) Init(bus core.EventBus, ctx core.Context) error {
	ctx.(PluginContext).Subscribe("dependency-event", func(core.Event) { p.deliveries++ })
	return nil
}
func (p *eventDependency) Stop(bus core.EventBus) error { p.stopBus = bus; return nil }

func TestExplicitRootEventBusAndContextReplacement(t *testing.T) {
	setupTestDB(t)
	oldBus, newBus, ignoredBus := newTestBus(), newTestBus(), newTestBus()
	root := core.NewContext(context.Background())
	root.SetService(core.ServiceEventBus, oldBus)
	p := &eventDependency{dependencyFixture: dependencyFixture{id: "watch-events", needs: []core.ServiceType{core.ServiceEventBus}}}
	m := NewManager(ignoredBus, root)
	m.builtInPlugins = []BuiltInPlugin{p}
	defer m.StopAll()
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	event := core.Event{Type: "dependency-event"}
	oldBus.Publish(event)
	if p.deliveries != 1 {
		t.Fatal("创建时未使用根上下文已注册的总线")
	}
	next := core.NewContext(context.Background())
	next.SetService(core.ServiceEventBus, newBus)
	m.SetCoreContext(next)
	if p.stopBus != oldBus {
		t.Fatal("旧插件停用收到错误总线")
	}
	oldBus.Publish(event)
	newBus.Publish(event)
	if p.deliveries != 2 || m.GetBus() != newBus {
		t.Fatal("更换上下文后仍监听旧总线")
	}
	empty := core.NewContext(context.Background())
	m.SetCoreContext(empty)
	if empty.GetEventBus() != newBus {
		t.Fatal("未将原总线注入缺少总线的新根上下文")
	}
	withoutBus := NewManager(nil)
	withoutBus.builtInPlugins = nil
	defer withoutBus.StopAll()
	withoutBus.SetCoreContext(next)
	if withoutBus.GetBus() != newBus {
		t.Fatal("无旧总线的管理器未采用新总线")
	}
}

func TestToggleOnlyExistingPluginFromCommittedConfiguration(t *testing.T) {
	db := setupTestDB(t)
	storePluginConfig(t, db, "plugin_status", `{"old":false}`)
	storePluginConfig(t, db, "custom_plugins", `[{"id":"cold","type":"script"}]`)
	m := NewManager(newTestBus())
	defer m.StopAll()
	if err := m.TogglePlugin("missing", true); !errors.Is(err, ErrPluginNotFound) {
		t.Fatal("写入了不存在的开关")
	}
	checkPluginConfig(t, db, "plugin_status", `{"old":false}`)
	if err := m.TogglePlugin("cold", false); err != nil {
		t.Fatal(err)
	}
	if m.IsPluginEnabled("cold") {
		t.Fatal("冷启动无法停用已存自定义插件")
	}
	if m.IsPluginEnabled("missing") {
		t.Error("不存在的插件被报告为启用")
	}
	if status, _ := m.GetPluginStatus("missing"); status != StateInactive {
		t.Error("不存在的插件被报告为运行中")
	}
}

func TestPanelOnlyWebhookNeedsNoEventBus(t *testing.T) {
	setupTestDB(t)
	m := NewManager(nil)
	defer m.StopAll()
	hook := fixturePanelPlugin()
	hook.Events = nil
	if err := m.AddOrUpdatePlugin(hook); err != nil {
		t.Fatal(err)
	}
	if _, runner, ok := m.PanelSnapshot(hook.ID); !ok || runner == nil {
		t.Fatal("纯面板因缺少总线而无法使用")
	}
	hook.Events = []string{"download.completed"}
	if err := m.AddOrUpdatePlugin(hook); err != nil {
		t.Fatal(err)
	}
	if state, _ := m.GetPluginStatus(hook.ID); state != StateSuspended || !m.IsPluginEnabled(hook.ID) {
		t.Fatalf("含事件的面板应保留开启意图并挂起: %s", state)
	}
	hook.Events = nil
	hook.Dependencies = []core.ServiceType{core.ServiceEventBus}
	if err := m.AddOrUpdatePlugin(hook); err != nil {
		t.Fatal(err)
	}
	if state, _ := m.GetPluginStatus(hook.ID); state != StateSuspended {
		t.Fatalf("显式声明事件总线的面板应挂起: %s", state)
	}
	m.SetCoreService(core.ServiceEventBus, newTestBus())
	if _, runner, ok := m.PanelSnapshot(hook.ID); !ok || runner == nil {
		t.Fatal("恢复事件总线后未唤醒面板")
	}
}

func TestRealCloudServiceGatesDependentPlugin(t *testing.T) {
	setupTestDB(t)
	m := NewManager(newTestBus())
	consumer := &dependencyFixture{id: "customer", needs: []core.ServiceType{Cloud115Service}}
	m.builtInPlugins = []BuiltInPlugin{consumer, &Cloud115Plugin{}}
	defer m.StopAll()
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if status, _ := m.GetPluginStatus(consumer.id); status != StateSuspended {
		t.Fatal("默认未启用的 115 不能满足依赖")
	}
	if err := m.Enable(Cloud115ID); err != nil {
		t.Fatal(err)
	}
	if status, _ := m.GetPluginStatus(consumer.id); status != StateActive {
		t.Fatal("实际云下载服务未唤醒消费者")
	}
	if err := m.Disable(Cloud115ID); err != nil {
		t.Fatal(err)
	}
	if m.IsPluginActive(consumer.id) || !m.IsPluginEnabled(consumer.id) {
		t.Fatal("停用实际提供者后消费者应保持启用意图并挂起")
	}
}

type brokenStopFixture struct{ dependencyFixture }

func (*brokenStopFixture) Stop(core.EventBus) error { panic("停用回调失败") }

func TestDependencyStopFailureDisposesAndReportsHealth(t *testing.T) {
	setupTestDB(t)
	m := NewManager(newTestBus())
	defer m.StopAll()
	p := &brokenStopFixture{dependencyFixture{id: "broken-stop", provides: "temporary"}}
	m.builtInPlugins = []BuiltInPlugin{p}
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if err := m.Disable(p.id); err != nil {
		t.Fatal(err)
	}
	st, msg := m.GetPluginStatus(p.id)
	if st != StateError || msg == "" || m.GetPluginContext(p.id) != nil || m.coreCtx.ServiceRevision("temporary") != 0 {
		t.Fatalf("停用异常未释放并上报: %s %s", st, msg)
	}
}
