package plugin

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// 测试 OnDispose 倒序 (LIFO) 执行与幂等性
func TestPluginContext_DisposableLIFOAndIdempotency(t *testing.T) {
	bus := newTestBus()
	pctx := NewPluginContext("test-plugin", bus, nil)

	var execOrder []int
	var mu sync.Mutex

	for i := 1; i <= 5; i++ {
		val := i
		pctx.OnDispose(func() {
			mu.Lock()
			defer mu.Unlock()
			execOrder = append(execOrder, val)
		})
	}

	// 首次 Dispose 执行
	pctx.Dispose()

	mu.Lock()
	if len(execOrder) != 5 {
		t.Fatalf("预期执行 5 个销毁钩子，实际执行了 %d 个", len(execOrder))
	}
	expected := []int{5, 4, 3, 2, 1}
	for i, v := range expected {
		if execOrder[i] != v {
			t.Errorf("销毁执行顺序不符合 LIFO: 索引 %d 期望 %d, 实际 %d (完整顺序: %v)", i, v, execOrder[i], execOrder)
		}
	}
	mu.Unlock()

	// 再次 Dispose 应幂等，不重复执行
	pctx.Dispose()
	mu.Lock()
	if len(execOrder) != 5 {
		t.Errorf("二次 Dispose 不应重复执行清理钩子: len=%d", len(execOrder))
	}
	mu.Unlock()
}

// 测试 OnDispose 钩子异常隔离（个别 panic 不影响后续清理）
func TestPluginContext_DisposablePanicIsolation(t *testing.T) {
	bus := newTestBus()
	pctx := NewPluginContext("test-plugin", bus, nil)

	var hook1Ran, hook3Ran bool

	pctx.OnDispose(func() {
		hook1Ran = true
	})
	pctx.OnDispose(func() {
		panic("boom in disposable hook!")
	})
	pctx.OnDispose(func() {
		hook3Ran = true
	})

	// Dispose 不应向外抛出 panic
	pctx.Dispose()

	if !hook1Ran || !hook3Ran {
		t.Errorf("异常清理钩子应被隔离: hook1=%v, hook3=%v", hook1Ran, hook3Ran)
	}
}

// 测试 Subscribe 自动注销与防止事件泄漏
func TestPluginContext_SubscribeAutoUnsubscribe(t *testing.T) {
	bus := newTestBus()
	pctx := NewPluginContext("test-plugin", bus, nil)

	var callCount atomic.Int32
	pctx.Subscribe("test.event", func(ev core.Event) {
		callCount.Add(1)
	})

	// 活跃期发布事件
	bus.Publish(core.Event{Type: "test.event", Time: time.Now()})
	if callCount.Load() != 1 {
		t.Fatalf("活跃期事件未触发: count=%d", callCount.Load())
	}

	// 销毁上下文
	pctx.Dispose()

	// 销毁后再次发布事件
	bus.Publish(core.Event{Type: "test.event", Time: time.Now()})
	if callCount.Load() != 1 {
		t.Errorf("销毁后事件监听应已自动注销: count=%d", callCount.Load())
	}
}

// 测试 Go 协程监管、上下文取消与 panic 隔离
func TestPluginContext_GoSupervisor(t *testing.T) {
	bus := newTestBus()
	pctx := NewPluginContext("test-plugin", bus, nil)

	var taskStopped atomic.Bool

	// 1. 正常后台任务
	pctx.Go(func(ctx context.Context) {
		<-ctx.Done()
		taskStopped.Store(true)
	})

	// 2. 发生 panic 的后台任务（应被内部 recover 隔离）
	pctx.Go(func(ctx context.Context) {
		panic("panic inside supervised goroutine")
	})

	time.Sleep(20 * time.Millisecond)

	// Dispose 触发上下文取消并等待后台任务退出
	pctx.Dispose()

	if !taskStopped.Load() {
		t.Errorf("后台协程应随 Dispose 取消并正常退出")
	}
}

// 测试动态服务注册与销毁时自动解绑
func TestPluginContext_ServiceRegistryLifecycle(t *testing.T) {
	baseCtx := core.NewContext(context.Background())
	bus := newTestBus()
	pctx := NewPluginContext("test-plugin", bus, baseCtx)

	type MyMockService struct {
		Name string
	}
	mockSvc := &MyMockService{Name: "plugin_service"}

	svcType := core.ServiceType("custom_mock_svc")
	pctx.RegisterService(svcType, mockSvc)

	// 验证在 pctx 与 baseCtx 中均可检索
	got, ok := pctx.GetService(svcType)
	if !ok || got.(*MyMockService).Name != "plugin_service" {
		t.Fatalf("未能从 pctx 中检索到注册的服务: %v", got)
	}

	gotBase, okBase := baseCtx.GetService(svcType)
	if !okBase || gotBase.(*MyMockService).Name != "plugin_service" {
		t.Fatalf("未能从 baseCtx 中检索到注册的服务: %v", gotBase)
	}

	// Dispose 后服务应自动解绑注销
	pctx.Dispose()

	if gotP, ok := pctx.GetService(svcType); ok || gotP != nil {
		t.Errorf("Dispose 后服务应从 pctx 中解绑: got=%v, ok=%v", gotP, ok)
	}
	if gotB, ok := baseCtx.GetService(svcType); ok && gotB != nil {
		t.Errorf("Dispose 后服务应从 baseCtx 中解绑: got=%v, ok=%v", gotB, ok)
	}
}

// 错误测试插件：Init 返回普通 error
type errorInitPlugin struct{}

func (p *errorInitPlugin) GetInfo() PluginInfo {
	return PluginInfo{
		ID:        "error_init_plugin",
		Name:      "Error Init Plugin",
		Version:   "1.0.0",
		IsBuiltIn: true,
	}
}

func (p *errorInitPlugin) Init(bus core.EventBus, ctx core.Context) error {
	return errors.New("initialization failed due to missing credentials")
}

func (p *errorInitPlugin) Stop(bus core.EventBus) error {
	return nil
}

// 测试 Manager 对插件详细健康状态 (PluginState) 与错误信息的追踪
func TestPluginManager_DetailedHealthAndErrorTracking(t *testing.T) {
	setupTestDB(t)
	bus := newTestBus()
	mgr := NewManager(bus)

	// 注册正常插件、panic 插件与返回 error 的插件
	mgr.builtInPlugins = append(mgr.builtInPlugins, &panicPlugin{}, &errorInitPlugin{})

	mgr.Load()

	// 1. 验证 panicPlugin 状态为 StateError 并记录堆栈
	st, errStr := mgr.GetPluginStatus("panic_plugin")
	if st != StateError {
		t.Errorf("panic_plugin 状态应为 StateError，实际: %v", st)
	}
	if errStr == "" {
		t.Errorf("panic_plugin 错误信息不应为空")
	}

	// 2. 验证 errorInitPlugin 状态为 StateError 并记录错误文本
	stErr, errStr2 := mgr.GetPluginStatus("error_init_plugin")
	if stErr != StateError {
		t.Errorf("error_init_plugin 状态应为 StateError，实际: %v", stErr)
	}
	if errStr2 != "initialization failed due to missing credentials" {
		t.Errorf("error_init_plugin 错误信息不匹配: %s", errStr2)
	}

	// 3. 验证正常插件状态为 StateActive 且无错误
	stActive, errActive := mgr.GetPluginStatus("metadata-mapping")
	if stActive != StateActive {
		t.Errorf("metadata-mapping 状态应为 StateActive，实际: %v", stActive)
	}
	if errActive != "" {
		t.Errorf("metadata-mapping 错误信息应为空: %s", errActive)
	}

	// 4. 验证停用插件状态为 StateInactive
	stInactive, _ := mgr.GetPluginStatus("yuc_schedule")
	if stInactive != StateInactive {
		t.Errorf("yuc_schedule 状态应为 StateInactive，实际: %v", stInactive)
	}

	// 5. 验证 GetPluginList() 输出了 Status 与 Error 字段
	list := mgr.GetPluginList()
	foundPanic := false
	for _, p := range list {
		if p.ID == "panic_plugin" {
			foundPanic = true
			if p.Status != StateError || p.Error == "" {
				t.Errorf("GetPluginList 中 panic_plugin 未包含健康信息: %+v", p)
			}
		}
	}
	if !foundPanic {
		t.Errorf("GetPluginList 中未检索到 panic_plugin")
	}

	// 6. 验证非法 Webhook URL 状态标记为 StateError
	invalidWebhook := PluginInfo{
		ID:        "invalid-webhook",
		Name:      "Invalid Webhook",
		Type:      "webhook",
		URL:       "http://127.0.0.1:9999/hook",
		Events:    []string{core.EventDownloadCompleted},
		IsBuiltIn: false,
		Enabled:   true,
	}
	mgr.allowPrivateWebhookForTest = false // 拦截私网地址
	mgr.customPlugins = append(mgr.customPlugins, invalidWebhook)

	mgr.Load()

	stHook, errHook := mgr.GetPluginStatus("invalid-webhook")
	if stHook != StateError {
		t.Errorf("非法 Webhook 状态应为 StateError，实际: %v", stHook)
	}
	if errHook == "" {
		t.Errorf("非法 Webhook 错误信息不应为空")
	}

	mgr.StopAll()
}

// 测试高并发压测下的读写安全与内存稳定性
func TestPluginManager_ConcurrencyStress(t *testing.T) {
	setupTestDB(t)
	bus := newTestBus()
	mgr := NewManager(bus)
	mgr.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup

	// 8 个并发读取协程
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				_ = mgr.GetPluginList()
				_ = mgr.IsPluginEnabled("metadata-mapping")
				_, _ = mgr.GetPluginStatus("metadata-mapping")
				_ = mgr.GetPluginContext("metadata-mapping")
			}
		}()
	}

	// 4 个并发写 / 启停协程
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			toggle := false
			for ctx.Err() == nil {
				_ = mgr.TogglePlugin("yuc_schedule", toggle)
				toggle = !toggle
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	// 2 个发布事件协程
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				bus.Publish(core.Event{
					Type: core.EventSubscriptionAdded,
					Payload: map[string]interface{}{
						"subscription_id": 1,
					},
					Time: time.Now(),
				})
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
	mgr.StopAll()
}
