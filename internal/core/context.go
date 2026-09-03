// Package core 提供核心接口和通用类型
package core

import (
	"context"
	"sync"
)

// universalContext 万能上下文实现
type universalContext struct {
	context.Context
	services map[ServiceType]interface{}
	mu       sync.RWMutex
}

// NewContext 创建新的万能上下文
func NewContext(ctx context.Context) Context {
	return &universalContext{
		Context:  ctx,
		services: make(map[ServiceType]interface{}),
	}
}

// GetService 根据类型获取服务实例
func (c *universalContext) GetService(t ServiceType) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	svc, ok := c.services[t]
	return svc, ok
}

// SetService 注册服务实例
func (c *universalContext) SetService(t ServiceType, svc interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.services[t] = svc
}

// MustGetService 获取服务，不存在则 panic
func (c *universalContext) MustGetService(t ServiceType) interface{} {
	svc, ok := c.GetService(t)
	if !ok {
		panic("service not found: " + string(t))
	}
	return svc
}

// GetDB 获取数据库实例
func (c *universalContext) GetDB() interface{} {
	svc, _ := c.GetService(ServiceDB)
	return svc
}

// GetConfig 获取配置实例
func (c *universalContext) GetConfig() interface{} {
	svc, _ := c.GetService(ServiceConfig)
	return svc
}

// GetEventBus 获取事件总线
func (c *universalContext) GetEventBus() EventBus {
	svc, _ := c.GetService(ServiceEventBus)
	if svc == nil {
		return nil
	}
	return svc.(EventBus)
}

// GetPluginManager 获取插件管理器
func (c *universalContext) GetPluginManager() interface{} {
	svc, _ := c.GetService(ServicePluginManager)
	return svc
}

// GetOrganizer 获取整理器
func (c *universalContext) GetOrganizer() Organizer {
	svc, _ := c.GetService(ServiceOrganizer)
	if svc == nil {
		return nil
	}
	return svc.(Organizer)
}

// GetDownloader 获取下载器
func (c *universalContext) GetDownloader() Downloader {
	svc, _ := c.GetService(ServiceDownloader)
	if svc == nil {
		return nil
	}
	return svc.(Downloader)
}

// GetSource 获取资源源
func (c *universalContext) GetSource() Source {
	svc, _ := c.GetService(ServiceSource)
	if svc == nil {
		return nil
	}
	return svc.(Source)
}

// GetMetadataProvider 获取元数据提供者
func (c *universalContext) GetMetadataProvider() MetadataProvider {
	svc, _ := c.GetService(ServiceMetadata)
	if svc == nil {
		return nil
	}
	return svc.(MetadataProvider)
}

// GetAIClassifier 获取 AI 分类器
func (c *universalContext) GetAIClassifier() AIClassifier {
	svc, _ := c.GetService(ServiceAI)
	if svc == nil {
		return nil
	}
	return svc.(AIClassifier)
}

// GetScheduler 获取调度器
func (c *universalContext) GetScheduler() interface{} {
	svc, _ := c.GetService(ServiceScheduler)
	return svc
}

// GetNotifier 获取通知管理器
func (c *universalContext) GetNotifier() interface{} {
	svc, _ := c.GetService(ServiceNotifier)
	return svc
}

// GetBackupManager 获取备份管理器
func (c *universalContext) GetBackupManager() interface{} {
	svc, _ := c.GetService(ServiceBackupManager)
	return svc
}

// GetTaskParser 获取任务解析器
func (c *universalContext) GetTaskParser() TaskParser {
	svc, _ := c.GetService(ServiceTaskParser)
	if svc == nil {
		return nil
	}
	return svc.(TaskParser)
}

// GetSearch 获取搜索引擎
func (c *universalContext) GetSearch() interface{} {
	svc, _ := c.GetService(ServiceSearch)
	return svc
}