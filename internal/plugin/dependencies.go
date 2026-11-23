package plugin

import (
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

var ErrPluginNotFound = errors.New("插件不存在")

func currentEventBus(registry core.ServiceRegistry) (core.EventBus, uint64) {
	service, revision := core.ReadService(registry, core.ServiceEventBus)
	bus, ok := service.(core.EventBus)
	if !ok || revision == 0 {
		return nil, 0
	}
	return bus, revision
}

func registeredEventBus(registry core.ServiceRegistry) core.EventBus {
	bus, _ := currentEventBus(registry)
	return bus
}

func (m *Manager) newScope(id string) *pluginContextImpl {
	bus, revision := currentEventBus(m.coreCtx)
	m.bus = bus
	scope := NewPluginContext(id, bus, m.coreCtx).(*pluginContextImpl)
	scope.busRevision = revision
	return scope
}

func (m *Manager) dependencyRevision(name core.ServiceType) uint64 {
	if name == core.ServiceEventBus {
		_, revision := currentEventBus(m.coreCtx)
		return revision
	}
	return m.coreCtx.ServiceRevision(name)
}

func rootEventBus(registry core.ServiceRegistry, fallback core.EventBus) core.EventBus {
	if bus := registeredEventBus(registry); bus != nil {
		return bus
	}
	registry.SetService(core.ServiceEventBus, fallback)
	return registeredEventBus(registry)
}

// SetCoreService 在运行中发布、替换或撤销宿主服务，并同步协调依赖。
func (m *Manager) SetCoreService(name core.ServiceType, service any) {
	m.lockChange()
	defer m.unlockChange()
	if m.stopDone != nil {
		return
	}
	if name == core.ServiceEventBus {
		bus, _ := service.(core.EventBus)
		m.coreCtx.SetService(name, bus)
		if m.coreCtx.ServiceRevision(name) == 0 {
			bus = nil
		}
		m.bus = bus
	} else {
		m.coreCtx.SetService(name, service)
	}
	if m.activePlugins != nil {
		m.applyRuntimeLocked()
	}
}

// 插件可能在初始化以外的协程注册服务；将并发变更合并，再读取当前注册表。
func (m *Manager) scheduleDependencyRefresh() {
	if !m.refreshQueued.CompareAndSwap(false, true) {
		return
	}
	go func() {
		m.lockChange()
		defer m.unlockChange()
		m.refreshQueued.Store(false)
		if m.stopDone == nil && m.activePlugins != nil {
			m.applyRuntimeLocked()
		}
	}()
}

func (m *Manager) builtinDependencies(p BuiltInPlugin) []core.ServiceType {
	var deps []core.ServiceType
	if d, ok := p.(DependencyPlugin); ok {
		deps = distinctDependencies(d.Dependencies())
	}
	if m.observedBus[p.GetInfo().ID] && !slices.Contains(deps, core.ServiceEventBus) {
		deps = append(deps, core.ServiceEventBus)
	}
	return deps
}
func distinctDependencies(deps []core.ServiceType) []core.ServiceType {
	var result []core.ServiceType
	for _, name := range deps {
		name = core.ServiceType(strings.TrimSpace(string(name)))
		if name != "" && !slices.Contains(result, name) {
			result = append(result, name)
		}
	}
	return result
}

func webhookDependencies(p PluginInfo) []core.ServiceType {
	deps := distinctDependencies(p.Dependencies)
	if !slices.Contains(deps, core.ServiceEventBus) && slices.ContainsFunc(p.Events, func(event string) bool { return strings.TrimSpace(event) != "" }) {
		deps = append(deps, core.ServiceEventBus)
	}
	return deps
}
func (m *Manager) missingServices(deps []core.ServiceType) []core.ServiceType {
	var missing []core.ServiceType
	for _, dep := range deps {
		if m.dependencyRevision(dep) == 0 && !slices.Contains(missing, dep) {
			missing = append(missing, dep)
		}
	}
	return missing
}
func (m *Manager) bindingsCurrent(id string, deps []core.ServiceType) bool {
	if len(deps) != len(m.bindings[id]) {
		return false
	}
	for _, dep := range deps {
		if v := m.dependencyRevision(dep); v == 0 || m.bindings[id][dep] != v {
			return false
		}
	}
	return true
}
func (m *Manager) recordActivation(id string, deps []core.ServiceType) {
	m.forgetActivation(id)
	m.activationOrder = append(m.activationOrder, id)
	bound := make(map[core.ServiceType]uint64, len(deps))
	for _, dep := range deps {
		if scope, ok := m.pluginContexts[id].(*pluginContextImpl); ok && dep == core.ServiceEventBus {
			bound[dep] = scope.busRevision
		} else {
			bound[dep] = m.dependencyRevision(dep)
		}
	}
	m.bindings[id] = bound
}
func (m *Manager) forgetActivation(id string) {
	m.activationOrder = slices.DeleteFunc(m.activationOrder, func(item string) bool { return item == id })
	delete(m.bindings, id)
}
func (m *Manager) builtinsByID() map[string]BuiltInPlugin {
	result := make(map[string]BuiltInPlugin, len(m.builtInPlugins))
	for _, p := range m.builtInPlugins {
		result[p.GetInfo().ID] = p
	}
	return result
}

// 首先沿实际持有的服务版本，找出将失效的全部消费方，然后按启动逆序停用。
func (m *Manager) invalidBuiltins(plugins map[string]BuiltInPlugin) map[string]bool {
	invalid := make(map[string]bool)
	owners := make(map[core.ServiceType]string)
	for id, p := range plugins {
		if m.pluginStates[id] != StateActive {
			continue
		}
		if scope, ok := m.pluginContexts[id].(*pluginContextImpl); ok && scope.usesBus.Load() && !m.observedBus[id] {
			m.observedBus[id] = true
			m.bindings[id][core.ServiceEventBus] = scope.busRevision
		}
		if !m.enabledLocked(id, !isDefaultDisabled(id)) || m.pluginContexts[id] == nil || m.pluginContexts[id].Err() != nil || !m.bindingsCurrent(id, m.builtinDependencies(p)) {
			invalid[id] = true
		}
		if ctx, ok := m.pluginContexts[id].(*pluginContextImpl); ok {
			for _, name := range ctx.ownedServices(m.coreCtx) {
				owners[name] = id
			}
		}
	}
	for {
		previous := len(invalid)
		for id, p := range plugins {
			if invalid[id] || m.pluginStates[id] != StateActive {
				continue
			}
			for _, dep := range m.builtinDependencies(p) {
				if invalid[owners[dep]] {
					invalid[id] = true
					break
				}
			}
		}
		if len(invalid) == previous {
			break
		}
	}
	for id, hook := range m.runningWebhooks {
		for _, dep := range webhookDependencies(hook) {
			if invalid[owners[dep]] {
				invalid[id] = true
				break
			}
		}
	}
	return invalid
}

func (m *Manager) reconcileBuiltinsLocked() {
	plugins := m.builtinsByID()
	failed := make(map[string]bool)
	// 通常每轮至少唤醒一个新提供方。反复覆盖彼此服务的环也必须有界。
	for pass := 0; pass <= 2*len(plugins)+1; pass++ {
		stale := m.invalidBuiltins(plugins)
		for _, id := range slices.Backward(slices.Clone(m.activationOrder)) {
			if stale[id] {
				if err := m.stopActiveLocked(id, plugins[id]); err != nil {
					failed[id] = true
				}
			}
		}
		started := false
		for _, p := range m.builtInPlugins {
			id := p.GetInfo().ID
			if m.draining[id] != nil {
				m.pluginStates[id] = StateDisposing
				continue
			}
			if failed[id] {
				continue
			}
			if !m.enabledLocked(id, !isDefaultDisabled(id)) {
				m.pluginStates[id] = StateInactive
				delete(m.pluginErrors, id)
				continue
			}
			if m.pluginStates[id] == StateActive {
				continue
			}
			deps := m.builtinDependencies(p)
			if len(m.missingServices(deps)) != 0 {
				m.pluginStates[id] = StateSuspended
				delete(m.pluginErrors, id)
				continue
			}
			if pass == 2*len(plugins)+1 {
				m.pluginStates[id] = StateError
				m.pluginErrors[id] = "插件反复替换它们依赖的服务"
				continue
			}
			if m.coreCtx.Err() != nil {
				m.pluginStates[id] = StateError
				m.pluginErrors[id] = "核心上下文已取消"
				continue
			}
			m.startBuiltin(p, deps)
			if m.pluginStates[id] != StateActive {
				failed[id] = true
			} else {
				started = true
			}
		}
		if !started {
			return
		}
	}
}

func (m *Manager) startBuiltin(p BuiltInPlugin, deps []core.ServiceType) {
	id := p.GetInfo().ID
	ctx := m.newScope(id)
	m.pluginStates[id] = StateLoading
	m.pluginContexts[id] = ctx
	// 记录初始化开始时所见版本，避免将初始化中发生的服务替换误作已有绑定。
	m.recordActivation(id, deps)
	run := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("crash during plugin init: %v", r)
			}
		}()
		return p.Init(m.bus, ctx)
	}
	var err error
	m.outsideStateLock(func() { err = run() })
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		m.pluginStates[id], m.pluginErrors[id] = StateError, err.Error()
		log.Printf("插件 %s 启动失败: %v", id, err)
		m.disposeScopeLocked(id, ctx)
		delete(m.pluginContexts, id)
		m.forgetActivation(id)
		return
	}
	scope := ctx
	scope.watchServices(m.scheduleDependencyRefresh)
	if scope.usesBus.Load() {
		m.observedBus[id] = true
		m.bindings[id][core.ServiceEventBus] = scope.busRevision
	} else if m.observedBus[id] {
		delete(m.observedBus, id)
		if !slices.Contains(m.builtinDependencies(p), core.ServiceEventBus) {
			delete(m.bindings[id], core.ServiceEventBus)
		}
	}
	m.pluginStates[id] = StateActive
	delete(m.pluginErrors, id)
}

func stopBuiltin(p BuiltInPlugin, bus core.EventBus) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("插件停用异常: %v", r)
		}
		if err != nil {
			log.Printf("插件 %s 停用失败: %v", p.GetInfo().ID, err)
		}
	}()
	return p.Stop(bus)
}

func (m *Manager) stopActiveLocked(id string, p BuiltInPlugin) error {
	m.pluginStates[id] = StateDisposing
	ctx := m.pluginContexts[id]
	bus := m.bus
	if c, ok := ctx.(*pluginContextImpl); ok {
		c.cancelForStop()
		bus = c.EventBus()
	}
	var stopErr error
	if p != nil {
		m.outsideStateLock(func() { stopErr = stopBuiltin(p, bus) })
	}
	if ctx != nil {
		m.disposeScopeLocked(id, ctx)
	}
	delete(m.pluginContexts, id)
	m.forgetActivation(id)
	if stopErr != nil {
		m.pluginStates[id], m.pluginErrors[id] = StateError, stopErr.Error()
		return stopErr
	}
	m.pluginStates[id] = StateInactive
	delete(m.pluginErrors, id)
	return nil
}
