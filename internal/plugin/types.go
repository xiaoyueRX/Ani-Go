package plugin

import (
	"context"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// PluginState Ani-Go 插件的运行状态
type PluginState string

const (
	StateSuspended PluginState = "suspended" // 已启用，等待依赖服务
	StateInactive  PluginState = "inactive"  // 未启用 / 已休眠
	StateLoading   PluginState = "loading"   // 正在初始化加载中
	StateActive    PluginState = "active"    // 正常激活运行中
	StateError     PluginState = "error"     // 启动或运行时发生异常，已熔断隔离
	StateDisposing PluginState = "disposing" // 正在销毁并释放资源
)

// PluginInfo 插件元数据契约，包含生命周期健康状态与错误信息
type PluginInfo struct {
	Panel               *Panel             `json:"panel,omitempty"`
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	Description         string             `json:"description"`
	Version             string             `json:"version"`
	Author              string             `json:"author"`
	AuthorURL           string             `json:"author_url"`
	Icon                string             `json:"icon"` // Lucide 图标名称或 URL
	Enabled             bool               `json:"enabled"`
	IsBuiltIn           bool               `json:"is_builtin"`
	Origin              string             `json:"origin"` // 服务端判定的维护来源
	Type                string             `json:"type"`   // 内置插件或可导入的事件 Webhook
	URL                 string             `json:"url,omitempty"`
	Secret              string             `json:"secret,omitempty"`
	Dependencies        []core.ServiceType `json:"dependencies,omitempty"`
	MissingDependencies []core.ServiceType `json:"missing_dependencies,omitempty"`
	Events              []string           `json:"events"`
	Status              PluginState        `json:"status,omitempty"` // 插件的实际运行状态
	Error               string             `json:"error,omitempty"`  // 最近错误信息（如有）
	LoadedAt            *time.Time         `json:"loaded_at,omitempty"`
}

// DisposableFunc 资源清理销毁函数
type DisposableFunc func()

// PluginContext 为 Ani-Go 插件登记受生命周期管理的资源、服务和日志
type PluginContext interface {
	core.Context // 继承核心万能上下文，获得服务发现与基础核心资源能力

	// PluginID 当前插件全局唯一标识
	PluginID() string

	// EventBus 获取绑定的事件总线
	EventBus() core.EventBus

	// Context 获取与插件生命周期绑定的标准上下文（插件停用时自动 Cancel）
	Context() context.Context

	// OnDispose 注册资源清理回调（在插件停用或销毁时倒序 LIFO 执行）
	OnDispose(fn DisposableFunc)

	// Subscribe 订阅事件并在插件注销/停用时自动取消订阅
	Subscribe(eventType string, handler core.EventHandler) core.SubscriptionID

	// Go 启动受监管的后台协程，随插件停用自动取消 context 并通过 WaitGroup 等待退出，自带 panic 隔离
	Go(task func(ctx context.Context))

	// RegisterService 动态注册服务至核心服务注册表，插件停用时自动解绑
	RegisterService(name core.ServiceType, svc interface{})

	// Scoped Logger 作用域日志记录器
	Logf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})

	// Dispose 执行所有注册的清理函数并取消上下文（幂等且并发安全）
	Dispose()
}

// BuiltInPlugin 内建插件生命周期接口（同时兼容传统与现代 PluginContext 调用模式）
type BuiltInPlugin interface {
	GetInfo() PluginInfo
	Init(bus core.EventBus, ctx core.Context) error
	Stop(bus core.EventBus) error
}

// DependencyPlugin 可选地声明启动所需的服务，原有插件无需实现。
type DependencyPlugin interface{ Dependencies() []core.ServiceType }
