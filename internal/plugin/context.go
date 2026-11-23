package plugin

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

type subscriptionRef struct {
	evType string
	subID  core.SubscriptionID
}

// pluginContextImpl 实现 PluginContext 接口，提供 DSH/Cordis 规范的生命周期管理与服务治理
type pluginContextImpl struct {
	coreCtx     core.Context
	pluginID    string
	bus         core.EventBus
	ctx         context.Context
	cancel      context.CancelFunc
	baseCoreCtx core.ServiceRegistry

	usesBus            atomic.Bool
	busRevision        uint64
	mu                 sync.Mutex
	disposed           bool
	cleaning           bool
	finished           chan struct{}
	disposables        []DisposableFunc
	subscriptions      []subscriptionRef
	registeredServices map[core.ServiceType]serviceRegistration
	onServiceChange    func()
	wg                 sync.WaitGroup
}

// NewPluginContext 创建绑定到指定插件生命周期的 PluginContext 实例
func NewPluginContext(pluginID string, bus core.EventBus, baseCtx core.Context) PluginContext {
	var parentCtx context.Context = context.Background()
	if baseCtx != nil {
		parentCtx = baseCtx
	}
	ctx, cancel := context.WithCancel(parentCtx)

	cCtx := core.NewContext(ctx)
	if bus != nil {
		cCtx.SetService(core.ServiceEventBus, bus)
	}
	if baseCtx != nil {
		if db := baseCtx.GetDB(); db != nil {
			cCtx.SetService(core.ServiceDB, db)
		}
		if cfg := baseCtx.GetConfig(); cfg != nil {
			cCtx.SetService(core.ServiceConfig, cfg)
		}
	} else if database.DB != nil {
		cCtx.SetService(core.ServiceDB, database.DB)
	}

	var registry core.ServiceRegistry
	if baseCtx != nil {
		registry = core.TrackServices(baseCtx)
	}
	return &pluginContextImpl{
		coreCtx:            cCtx,
		pluginID:           pluginID,
		bus:                bus,
		ctx:                ctx,
		cancel:             cancel,
		baseCoreCtx:        registry,
		registeredServices: make(map[core.ServiceType]serviceRegistration),
		finished:           make(chan struct{}),
	}
}

// PluginID 返回当前插件唯一标识
func (c *pluginContextImpl) PluginID() string {
	return c.pluginID
}

// EventBus 返回当前上下文绑定的事件总线
func (c *pluginContextImpl) EventBus() core.EventBus {
	if c.bus != nil {
		return c.bus
	}
	return c.coreCtx.GetEventBus()
}

// Context 返回与插件生命周期绑定的标准上下文
func (c *pluginContextImpl) Context() context.Context {
	return c.ctx
}

// context.Context 接口实现
func (c *pluginContextImpl) Deadline() (deadline time.Time, ok bool) {
	return c.ctx.Deadline()
}

func (c *pluginContextImpl) Done() <-chan struct{} {
	return c.ctx.Done()
}

func (c *pluginContextImpl) Err() error {
	return c.ctx.Err()
}

func (c *pluginContextImpl) Value(key interface{}) interface{} {
	return c.ctx.Value(key)
}

// core.Context 接口实现
func (c *pluginContextImpl) GetService(t core.ServiceType) (interface{}, bool) {
	c.mu.Lock()
	disposed := c.disposed
	c.mu.Unlock()
	if disposed {
		return nil, false
	}

	if svc, ok := c.coreCtx.GetService(t); ok && svc != nil {
		return svc, true
	}
	if c.baseCoreCtx != nil {
		if svc, ok := c.baseCoreCtx.GetService(t); ok && svc != nil {
			return svc, true
		}
	}
	return nil, false
}

func (c *pluginContextImpl) SetService(t core.ServiceType, svc interface{}) {
	c.coreCtx.SetService(t, svc)
}

func (c *pluginContextImpl) MustGetService(t core.ServiceType) interface{} {
	svc, ok := c.GetService(t)
	if !ok {
		panic("service not found: " + string(t))
	}
	return svc
}

func (c *pluginContextImpl) GetDB() interface{} {
	if c.baseCoreCtx != nil {
		if db := c.baseCoreCtx.GetDB(); db != nil {
			return db
		}
	}
	if db := c.coreCtx.GetDB(); db != nil {
		return db
	}
	return database.DB
}

func (c *pluginContextImpl) GetConfig() interface{} {
	if c.baseCoreCtx != nil {
		if cfg := c.baseCoreCtx.GetConfig(); cfg != nil {
			return cfg
		}
	}
	return c.coreCtx.GetConfig()
}

func (c *pluginContextImpl) GetEventBus() core.EventBus {
	return c.EventBus()
}

func (c *pluginContextImpl) GetPluginManager() interface{} {
	if c.baseCoreCtx != nil {
		return c.baseCoreCtx.GetPluginManager()
	}
	return c.coreCtx.GetPluginManager()
}

func (c *pluginContextImpl) GetOrganizer() core.Organizer {
	if c.baseCoreCtx != nil {
		return c.baseCoreCtx.GetOrganizer()
	}
	return c.coreCtx.GetOrganizer()
}

func (c *pluginContextImpl) GetDownloader() core.Downloader {
	if c.baseCoreCtx != nil {
		return c.baseCoreCtx.GetDownloader()
	}
	return c.coreCtx.GetDownloader()
}

func (c *pluginContextImpl) GetSource() core.Source {
	if c.baseCoreCtx != nil {
		return c.baseCoreCtx.GetSource()
	}
	return c.coreCtx.GetSource()
}

func (c *pluginContextImpl) GetMetadataProvider() core.MetadataProvider {
	if c.baseCoreCtx != nil {
		return c.baseCoreCtx.GetMetadataProvider()
	}
	return c.coreCtx.GetMetadataProvider()
}

func (c *pluginContextImpl) GetAIClassifier() core.AIClassifier {
	if c.baseCoreCtx != nil {
		return c.baseCoreCtx.GetAIClassifier()
	}
	return c.coreCtx.GetAIClassifier()
}

func (c *pluginContextImpl) GetScheduler() interface{} {
	if c.baseCoreCtx != nil {
		return c.baseCoreCtx.GetScheduler()
	}
	return c.coreCtx.GetScheduler()
}

func (c *pluginContextImpl) GetNotifier() interface{} {
	if c.baseCoreCtx != nil {
		return c.baseCoreCtx.GetNotifier()
	}
	return c.coreCtx.GetNotifier()
}

func (c *pluginContextImpl) GetBackupManager() interface{} {
	if c.baseCoreCtx != nil {
		return c.baseCoreCtx.GetBackupManager()
	}
	return c.coreCtx.GetBackupManager()
}

func (c *pluginContextImpl) GetTaskParser() core.TaskParser {
	if c.baseCoreCtx != nil {
		return c.baseCoreCtx.GetTaskParser()
	}
	return c.coreCtx.GetTaskParser()
}

func (c *pluginContextImpl) GetSearch() interface{} {
	if c.baseCoreCtx != nil {
		return c.baseCoreCtx.GetSearch()
	}
	return c.coreCtx.GetSearch()
}

type serviceRegistration struct {
	revision uint64
	release  func()
}

// RegisterService 的登记和销毁共享同一把锁，旧注册只能注销自己的版本。
func (c *pluginContextImpl) RegisterService(name core.ServiceType, svc interface{}) {
	c.mu.Lock()
	if c.disposed || c.ctx.Err() != nil {
		c.mu.Unlock()
		return
	}
	if old, ok := c.registeredServices[name]; ok {
		old.release()
		delete(c.registeredServices, name)
	}
	if svc != nil {
		local := core.TrackServices(c.coreCtx)
		version, releaseLocal := local.BindService(name, svc)
		release := releaseLocal
		if c.baseCoreCtx != nil {
			remoteVersion, releaseRemote := c.baseCoreCtx.BindService(name, svc)
			version = remoteVersion
			release = func() { releaseRemote(); releaseLocal() }
		}
		c.registeredServices[name] = serviceRegistration{version, release}
	}
	notify := c.onServiceChange
	c.mu.Unlock()
	if notify != nil {
		notify()
	}
}

func (c *pluginContextImpl) watchServices(notify func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onServiceChange = notify
}

// cancelForStop 让 Stop 可以等待受管工作退出，同时保留服务至 Dispose。
func (c *pluginContextImpl) cancelForStop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onServiceChange = nil
	c.cancel()
}

func (c *pluginContextImpl) ownedServices(registry core.ServiceRegistry) []core.ServiceType {
	c.mu.Lock()
	defer c.mu.Unlock()
	var result []core.ServiceType
	for name, reg := range c.registeredServices {
		if registry.ServiceRevision(name) == reg.revision {
			result = append(result, name)
		}
	}
	return result
}

// OnDispose 注册销毁清理函数，插件停用时倒序 (LIFO) 执行
func (c *pluginContextImpl) OnDispose(fn DisposableFunc) {
	if fn == nil {
		return
	}
	c.mu.Lock()
	if !c.cleaning {
		c.disposables = append(c.disposables, fn)
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	c.runCleanup(fn)
}

func (c *pluginContextImpl) runCleanup(fn DisposableFunc) {
	defer func() {
		if issue := recover(); issue != nil {
			c.Errorf("清理回调异常: %v", issue)
		}
	}()
	fn()
}

func (c *pluginContextImpl) beginWork() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disposed || c.ctx.Err() != nil {
		return false
	}
	c.wg.Add(1)
	return true
}

// Subscribe 订阅事件总线并在插件销毁时自动注销，杜绝事件句柄泄漏
func (c *pluginContextImpl) Subscribe(eventType string, handler core.EventHandler) core.SubscriptionID {
	if handler == nil {
		return 0
	}
	c.mu.Lock()
	if c.disposed || c.ctx.Err() != nil {
		c.mu.Unlock()
		return 0
	}
	first := !c.usesBus.Swap(true)
	notify := c.onServiceChange
	c.mu.Unlock()
	if first && notify != nil {
		notify()
	}
	if c.bus == nil {
		return 0
	}

	subID := c.bus.Subscribe(eventType, func(ev core.Event) {
		if !c.beginWork() {
			return
		}
		defer c.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				c.Errorf("事件处理器捕获异常崩溃 [%s]: %v", eventType, r)
			}
		}()
		handler(ev)
	})

	c.mu.Lock()
	if c.disposed || c.ctx.Err() != nil {
		c.mu.Unlock()
		c.bus.Unsubscribe(eventType, subID)
		return 0
	}
	c.subscriptions = append(c.subscriptions, subscriptionRef{evType: eventType, subID: subID})
	c.mu.Unlock()
	return subID
}

// Go 启动受监管的后台协程，随插件停用自动取消 context 并通过 WaitGroup 等待退出，自带 panic 隔离
func (c *pluginContextImpl) Go(task func(ctx context.Context)) {
	if task == nil {
		return
	}
	if !c.beginWork() {
		return
	}

	go func() {
		defer c.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				c.Errorf("后台协程捕获异常崩溃: %v", r)
			}
		}()
		task(c.ctx)
	}()
}

// Logf 记录插件作用域常规日志
func (c *pluginContextImpl) Logf(format string, args ...interface{}) {
	log.Printf("🔌 [插件 %s] "+fmt.Sprintf(format, args...), c.pluginID)
}

// Warnf 记录插件作用域告警日志
func (c *pluginContextImpl) Warnf(format string, args ...interface{}) {
	log.Printf("⚠️ [插件 %s] "+fmt.Sprintf(format, args...), c.pluginID)
}

// Errorf 记录插件作用域错误日志
func (c *pluginContextImpl) Errorf(format string, args ...interface{}) {
	log.Printf("❌ [插件 %s] "+fmt.Sprintf(format, args...), c.pluginID)
}

// Dispose 幂等执行所有注册的清理函数、注销事件与服务，并取消上下文
func (c *pluginContextImpl) Dispose() {
	c.mu.Lock()
	if c.disposed {
		c.mu.Unlock()
		return
	}
	c.disposed = true

	// 1. 取消绑定的 Context
	if c.cancel != nil {
		c.cancel()
	}

	subs := c.subscriptions
	c.subscriptions = nil
	services := c.registeredServices
	c.registeredServices = nil
	notify := c.onServiceChange
	c.onServiceChange = nil
	c.mu.Unlock()

	if c.bus != nil {
		for _, s := range subs {
			if s.subID != 0 {
				c.bus.Unsubscribe(s.evType, s.subID)
			}
		}
	}
	// 服务先从注册表撤销，旧调用持有的资源须等已进入的回调与工作者退出后才关闭。
	for _, reg := range services {
		reg.release()
	}
	if notify != nil && len(services) != 0 {
		notify()
	}
	go func() {
		defer close(c.finished)
		c.wg.Wait()
		c.mu.Lock()
		c.cleaning = true
		disposables := c.disposables
		c.disposables = nil
		c.mu.Unlock()
		for i := len(disposables) - 1; i >= 0; i-- {
			c.runCleanup(disposables[i])
		}
	}()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-c.finished:
	case <-timer.C:
		c.Warnf("等待在途工作退出，清理将在其结束后继续")
	}
}
