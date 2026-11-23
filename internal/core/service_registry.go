package core

import (
	"reflect"
	"slices"
	"sync"
)

// ServiceRegistry 为原有上下文补充服务版本与按注册所有权释放的能力。
type ServiceRegistry interface {
	Context
	BindService(ServiceType, any) (uint64, func())
	ServiceRevision(ServiceType) uint64
}

type trackedServices struct {
	Context
	mu       sync.RWMutex
	sequence uint64
	layers   map[ServiceType][]serviceLayer
}

// TrackServices 也可包装只实现原有 Context 接口的宿主。
func TrackServices(ctx Context) ServiceRegistry {
	if registry, ok := ctx.(ServiceRegistry); ok {
		return registry
	}
	return &trackedServices{Context: ctx, sequence: 1, layers: make(map[ServiceType][]serviceLayer)}
}

func availableService(v any) bool {
	if v == nil {
		return false
	}
	switch value := reflect.ValueOf(v); value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	}
	return true
}

type serviceLayer struct {
	revision uint64
	value    any
}

// SetService 是宿主的显式替换，所有之前的绑定都失去所有权。
func (r *trackedServices) SetService(name ServiceType, service any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !availableService(service) {
		delete(r.layers, name)
		r.Context.SetService(name, nil)
		return
	}
	r.sequence++
	r.layers[name] = []serviceLayer{{r.sequence, service}}
	r.Context.SetService(name, service)
}

// ReadService 同时取得实例和版本，避免把旧实例记录为刚替换的新版本。
func ReadService(registry ServiceRegistry, name ServiceType) (any, uint64) {
	if snapshot, ok := registry.(interface {
		ServiceSnapshot(ServiceType) (any, uint64)
	}); ok {
		return snapshot.ServiceSnapshot(name)
	}
	// 兼容已有注册表。先取版本让读取期间发生的替换能够触发重新绑定。
	revision := registry.ServiceRevision(name)
	value, _ := registry.GetService(name)
	return value, revision
}

func (r *trackedServices) ServiceRevision(name ServiceType) uint64 {
	_, revision := r.ServiceSnapshot(name)
	return revision
}

func (r *trackedServices) ServiceSnapshot(name ServiceType) (any, uint64) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	svc, ok := r.Context.GetService(name)
	if !ok || !availableService(svc) {
		return nil, 0
	}
	if layers := r.layers[name]; len(layers) > 0 {
		return svc, layers[len(layers)-1].revision
	}
	return svc, 1
}

// 后来的作用域临时覆盖仍在运行的提供者。撤销任意一层时只移除该次注册。
func (r *trackedServices) BindService(name ServiceType, svc any) (uint64, func()) {
	r.mu.Lock()
	layers := r.layers[name]
	if len(layers) == 0 {
		if previous, ok := r.Context.GetService(name); ok && availableService(previous) {
			layers = append(layers, serviceLayer{1, previous})
		}
	}
	r.sequence++
	version := r.sequence
	if !availableService(svc) {
		svc = nil
	}
	r.layers[name] = append(layers, serviceLayer{version, svc})
	r.Context.SetService(name, svc)
	r.mu.Unlock()
	return version, func() { r.release(name, version) }
}

func (r *trackedServices) release(name ServiceType, version uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	layers := r.layers[name]
	for i, layer := range layers {
		if layer.revision != version {
			continue
		}
		wasCurrent := i == len(layers)-1
		layers = slices.Delete(layers, i, i+1)
		if len(layers) == 0 {
			delete(r.layers, name)
			r.Context.SetService(name, nil)
		} else {
			r.layers[name] = layers
			if wasCurrent {
				r.Context.SetService(name, layers[len(layers)-1].value)
			}
		}
		return
	}
}
