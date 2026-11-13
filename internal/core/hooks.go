// Package core 提供核心接口和通用类型
package core

import (
	"context"
	"log"
	"sync"
)

// HookType 钩子类型
type HookType string

const (
	// HookNaming 命名钩子 - 允许插件拦截并修改整理后的文件名
	HookNaming HookType = "naming"
	// HookFilter 过滤钩子 - 允许插件动态干预 RSS 过滤逻辑
	HookFilter HookType = "filter"
	// HookParse 解析钩子 - 允许插件动态介入任务或标题解析
	HookParse HookType = "parse"
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

// ParseHookRegistration 解析拦截器注册条目
type ParseHookRegistration struct {
	Priority    HookPriority
	Interceptor ParseInterceptor
	Name        string
}

// WaterfallHookManager 瀑布流钩子与流水线拦截器管理器
type WaterfallHookManager struct {
	namingHooks []HookRegistration
	filterHooks []HookRegistration
	parseHooks  []ParseHookRegistration
	mu          sync.RWMutex
}

// NewWaterfallHookManager 创建钩子管理器
func NewWaterfallHookManager() *WaterfallHookManager {
	return &WaterfallHookManager{
		namingHooks: make([]HookRegistration, 0),
		filterHooks: make([]HookRegistration, 0),
		parseHooks:  make([]ParseHookRegistration, 0),
	}
}

// RegisterNamingHook 注册命名钩子（函数模式，兼容已有插件）
func (m *WaterfallHookManager) RegisterNamingHook(priority HookPriority, name string, handler HookFunc) {
	if m == nil || handler == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.namingHooks = append(m.namingHooks, HookRegistration{
		Type:     HookNaming,
		Priority: priority,
		Handler:  handler,
		Name:     name,
	})
	sortHooks(m.namingHooks)
	log.Printf("🪝 注册命名钩子: %s (优先级: %d)", name, priority)
}

// RegisterOrganizeInterceptor 注册强类型整理阶段流水线拦截器
func (m *WaterfallHookManager) RegisterOrganizeInterceptor(priority HookPriority, interceptor OrganizeInterceptor) {
	if m == nil || interceptor == nil {
		return
	}
	name := interceptor.Name()
	m.RegisterNamingHook(priority, name, func(ctx context.Context, input interface{}) (interface{}, error) {
		in, ok := input.(NamingHookInput)
		if !ok {
			return nil, nil
		}
		orgInput := OrganizeInput{
			Anime:        in.Anime,
			Episode:      in.Episode,
			Template:     in.Template,
			VarValues:    in.VarValues,
			ProposedPath: in.RenderedPath,
		}
		out, err := interceptor.InterceptOrganize(ctx, orgInput)
		if err != nil {
			return nil, err
		}
		return NamingHookOutput{
			RenderedPath: out.FinalPath,
			Cancel:       out.Cancel,
			Reason:       out.Reason,
		}, nil
	})
}

// RegisterFilterHook 注册过滤钩子（函数模式，兼容已有插件）
func (m *WaterfallHookManager) RegisterFilterHook(priority HookPriority, name string, handler HookFunc) {
	if m == nil || handler == nil {
		return
	}
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

// RegisterFilterInterceptor 注册强类型过滤流水线拦截器
func (m *WaterfallHookManager) RegisterFilterInterceptor(priority HookPriority, interceptor FilterInterceptor) {
	if m == nil || interceptor == nil {
		return
	}
	name := interceptor.Name()
	m.RegisterFilterHook(priority, name, func(ctx context.Context, input interface{}) (interface{}, error) {
		in, ok := input.(FilterHookInput)
		if !ok {
			return nil, nil
		}
		out, err := interceptor.InterceptFilter(ctx, FilterInput{
			Item:   in.Item,
			Filter: in.Filter,
		})
		if err != nil {
			return nil, err
		}
		return FilterHookOutput{
			Allow:  out.Allow,
			Reason: out.Reason,
		}, nil
	})
}

// RegisterParseInterceptor 注册任务与标题解析流水线拦截器
func (m *WaterfallHookManager) RegisterParseInterceptor(priority HookPriority, interceptor ParseInterceptor) {
	if m == nil || interceptor == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.parseHooks = append(m.parseHooks, ParseHookRegistration{
		Priority:    priority,
		Interceptor: interceptor,
		Name:        interceptor.Name(),
	})
	// 排序
	for i := 1; i < len(m.parseHooks); i++ {
		key := m.parseHooks[i]
		j := i - 1
		for j >= 0 && m.parseHooks[j].Priority > key.Priority {
			m.parseHooks[j+1] = m.parseHooks[j]
			j--
		}
		m.parseHooks[j+1] = key
	}
	log.Printf("🪝 注册解析拦截器: %s (优先级: %d)", interceptor.Name(), priority)
}

// sortHooks 按优先级排序钩子（升序，优先级数值小的在前）
func sortHooks(hooks []HookRegistration) {
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
	if m == nil {
		return input.RenderedPath, false, ""
	}

	m.mu.RLock()
	if len(m.namingHooks) == 0 {
		m.mu.RUnlock()
		return input.RenderedPath, false, ""
	}
	hooks := make([]HookRegistration, len(m.namingHooks))
	copy(hooks, m.namingHooks)
	m.mu.RUnlock()

	currentPath := input.RenderedPath
	for _, hook := range hooks {
		hookInput := NamingHookInput{
			Anime:        input.Anime,
			Episode:      input.Episode,
			Template:     input.Template,
			VarValues:    input.VarValues,
			RenderedPath: currentPath,
		}

		// 异常隔离：单个钩子 panic 不崩溃主进程
		var (
			output interface{}
			err    error
		)
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("⚠️ 命名钩子 [%s] 发生 panic 恢复: %v", hook.Name, r)
					output = nil
				}
			}()
			output, err = hook.Handler(ctx, hookInput)
		}()

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
	if m == nil {
		return true, ""
	}

	m.mu.RLock()
	if len(m.filterHooks) == 0 {
		m.mu.RUnlock()
		return true, ""
	}
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

		var (
			output interface{}
			err    error
		)
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("⚠️ 过滤钩子 [%s] 发生 panic 恢复: %v", hook.Name, r)
					output = nil
				}
			}()
			output, err = hook.Handler(ctx, hookInput)
		}()

		if err != nil {
			log.Printf("⚠️ 过滤钩子 [%s] 执行错误: %v", hook.Name, err)
			continue
		}

		if out, ok := output.(FilterHookOutput); ok {
			if !out.Allow {
				return false, out.Reason
			}
			if out.Reason != "" {
				reason = out.Reason
			}
		}
	}

	return allowed, reason
}

// ExecuteParseInterceptors 执行解析流水线拦截器链
// 返回最终解析结果与可能发生的错误
func (m *WaterfallHookManager) ExecuteParseInterceptors(ctx context.Context, rawInput string, initial ParseResult) (ParseResult, error) {
	if m == nil {
		return initial, nil
	}

	m.mu.RLock()
	if len(m.parseHooks) == 0 {
		m.mu.RUnlock()
		return initial, nil
	}
	hooks := make([]ParseHookRegistration, len(m.parseHooks))
	copy(hooks, m.parseHooks)
	m.mu.RUnlock()

	currentResult := initial
	for _, hook := range hooks {
		var (
			nextResult ParseResult
			err        error
		)
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("⚠️ 解析拦截器 [%s] 发生 panic 恢复: %v", hook.Name, r)
					nextResult = currentResult
				}
			}()
			nextResult, err = hook.Interceptor.InterceptParse(ctx, rawInput, currentResult)
		}()

		if err != nil {
			log.Printf("⚠️ 解析拦截器 [%s] 执行错误: %v", hook.Name, err)
			continue
		}

		// 若拦截器返回了更高置信度或有效更新，则采纳
		if nextResult.Confidence >= currentResult.Confidence || nextResult.Title != "" {
			currentResult = nextResult
		}
	}

	return currentResult, nil
}

// UnregisterHooksByName 根据名称注销所有类型的钩子与拦截器（支持热插拔）
func (m *WaterfallHookManager) UnregisterHooksByName(name string) {
	if m == nil {
		return
	}
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

	// 过滤解析拦截器
	newParse := make([]ParseHookRegistration, 0, len(m.parseHooks))
	for _, h := range m.parseHooks {
		if h.Name != name {
			newParse = append(newParse, h)
		}
	}
	m.parseHooks = newParse

	log.Printf("🪝 已注销插件 [%s] 的所有钩子与拦截器", name)
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
	SeriesID string
	Series   string
}
