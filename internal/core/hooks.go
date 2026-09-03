// Package core 提供核心接口和通用类型
package core

import (
	"context"
	"log"
	"sync"
)

type HookType string

const (
	// HookNaming 命名钩子 - 允许插件拦截并修改整理后的文件名
	HookNaming HookType = "naming"
	// HookFilter 过滤钩子 - 允许插件动态干预 RSS 过滤逻辑
	HookFilter HookType = "filter"
)

// HookPriority 钩子优先级，数值越小优先级越高
type HookPriority int

const (
	PriorityHighest HookPriority = 0
	PriorityHigh    HookPriority = 10
	PriorityNormal  HookPriority = 50
	PriorityLow     HookPriority = 100
	PriorityLowest  HookPriority = 200
)

// NamingHookInput 命名钩子输入
type NamingHookInput struct {
	Anime       Anime
	Episode     Episode
	Template    string
	VarValues   VarValues
	RenderedPath string
}

// NamingHookOutput 命名钩子输出
type NamingHookOutput struct {
	RenderedPath string
	Cancel       bool // 一票否决：true 表示取消整理
	Reason       string
}

// FilterHookInput 过滤钩子输入
type FilterHookInput struct {
	Item   TorrentItem
	Filter Filter
}

// FilterHookOutput 过滤钩子输出
type FilterHookOutput struct {
	Allow  bool   // 是否通过过滤
	Reason string // 拒绝原因
}

// HookFunc 钩子函数类型
type HookFunc func(ctx context.Context, input interface{}) (interface{}, error)

// HookRegistration 钩子注册信息
type HookRegistration struct {
	Type      HookType
	Priority  HookPriority
	Handler   HookFunc
	Name      string // 插件名称，用于调试
}

// WaterfallHookManager 瀑布流钩子管理器
type WaterfallHookManager struct {
	namingHooks []HookRegistration
	filterHooks []HookRegistration
	mu          sync.RWMutex
}

// NewWaterfallHookManager 创建钩子管理器
func NewWaterfallHookManager() *WaterfallHookManager {
	return &WaterfallHookManager{
		namingHooks: make([]HookRegistration, 0),
		filterHooks: make([]HookRegistration, 0),
	}
}

// RegisterNamingHook 注册命名钩子
func (m *WaterfallHookManager) RegisterNamingHook(priority HookPriority, name string, handler HookFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.namingHooks = append(m.namingHooks, HookRegistration{
		Type:     HookNaming,
		Priority: priority,
		Handler:  handler,
		Name:     name,
	})
	// 按优先级排序
	sortHooks(m.namingHooks)
	log.Printf("🪝 注册命名钩子: %s (优先级: %d)", name, priority)
}

// RegisterFilterHook 注册过滤钩子
func (m *WaterfallHookManager) RegisterFilterHook(priority HookPriority, name string, handler HookFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.filterHooks = append(m.filterHooks, HookRegistration{
		Type:     HookFilter,
		Priority: priority,
		Handler:  handler,
		Name:     name,
	})
	sortHooks(m.filterHooks)
	log.Printf("🪝 注册过滤钩子: %s (优先级: %d)", name, priority)
}

// sortHooks 按优先级排序钩子（升序，优先级数值小的在前）
func sortHooks(hooks []HookRegistration) {
	// 简单插入排序，钩子数量通常很少
	for i := 1; i < len(hooks); i++ {
		key := hooks[i]
		j := i - 1
		for j >= 0 && hooks[j].Priority > key.Priority {
			hooks[j+1] = hooks[j]
			j--
		}
		hooks[j+1] = key
	}
}

// ExecuteNamingHooks 执行命名钩子链（瀑布流模式）
// 返回最终路径，是否取消，取消原因
func (m *WaterfallHookManager) ExecuteNamingHooks(ctx context.Context, input NamingHookInput) (string, bool, string) {
	m.mu.RLock()
	hooks := make([]HookRegistration, len(m.namingHooks))
	copy(hooks, m.namingHooks)
	m.mu.RUnlock()

	currentPath := input.RenderedPath
	for _, hook := range hooks {
		hookInput := NamingHookInput{
			Anime:       input.Anime,
			Episode:     input.Episode,
			Template:    input.Template,
			VarValues:   input.VarValues,
			RenderedPath: currentPath,
		}

		output, err := hook.Handler(ctx, hookInput)
		if err != nil {
			log.Printf("⚠️ 命名钩子 [%s] 执行错误: %v", hook.Name, err)
			continue
		}

		if out, ok := output.(NamingHookOutput); ok {
			if out.Cancel {
				return "", true, out.Reason
			}
			if out.RenderedPath != "" {
				currentPath = out.RenderedPath
			}
		}
	}

	return currentPath, false, ""
}

// ExecuteFilterHooks 执行过滤钩子链（瀑布流模式）
// 返回是否允许通过，拒绝原因
func (m *WaterfallHookManager) ExecuteFilterHooks(ctx context.Context, input FilterHookInput) (bool, string) {
	m.mu.RLock()
	hooks := make([]HookRegistration, len(m.filterHooks))
	copy(hooks, m.filterHooks)
	m.mu.RUnlock()

	allowed := true
	var reason string

	for _, hook := range hooks {
		hookInput := FilterHookInput{
			Item:   input.Item,
			Filter: input.Filter,
		}

		output, err := hook.Handler(ctx, hookInput)
		if err != nil {
			log.Printf("⚠️ 过滤钩子 [%s] 执行错误: %v", hook.Name, err)
			continue
		}

		if out, ok := output.(FilterHookOutput); ok {
			if !out.Allow {
				return false, out.Reason
			}
			// 如果通过，更新 reason（最后一个通过的钩子的原因）
			if out.Reason != "" {
				reason = out.Reason
			}
		}
	}

	return allowed, reason
}

// UnregisterHooksByName 根据名称注销钩子（支持热插拔）
func (m *WaterfallHookManager) UnregisterHooksByName(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 过滤命名钩子
	newNaming := make([]HookRegistration, 0, len(m.namingHooks))
	for _, h := range m.namingHooks {
		if h.Name != name {
			newNaming = append(newNaming, h)
		}
	}
	m.namingHooks = newNaming

	// 过滤过滤钩子
	newFilter := make([]HookRegistration, 0, len(m.filterHooks))
	for _, h := range m.filterHooks {
		if h.Name != name {
			newFilter = append(newFilter, h)
		}
	}
	m.filterHooks = newFilter

	log.Printf("🪝 已注销插件 [%s] 的所有钩子", name)
}

type VarValues struct {
	TitleCN  string
	TitleEN  string
	Year     int
	Season   int
	Ep       float32
	Ext      string
	AnimeID  string
	Provider string
	TMDBID   string
	IMDBID   string
}
