package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

type customSubRef struct {
	evType string
	subID  core.SubscriptionID
}

// Manager 插件生命周期管理器，统一负责内建插件与自定义 Webhook 插件的加载、启停、健康追踪与热重载
type Manager struct {
	bus                        core.EventBus
	coreCtx                    core.Context // 核心系统万能上下文
	mu                         sync.RWMutex
	builtInPlugins             []BuiltInPlugin
	customPlugins              []PluginInfo
	activePlugins              map[string]bool          // id -> enabled
	pluginStates               map[string]PluginState   // id -> PluginState (active/inactive/error/loading/disposing)
	pluginErrors               map[string]string        // id -> 最近错误信息
	pluginContexts             map[string]PluginContext // id -> PluginContext
	customSubs                 map[string][]customSubRef
	webhookCtx                 context.Context
	webhookCancel              context.CancelFunc
	webhookWg                  sync.WaitGroup
	allowPrivateWebhookForTest bool // 仅限单元测试使用，允许连接回环/私网地址
}

// NewManager 创建插件管理器实例并注册全部内建插件，支持注入可选的核心上下文
func NewManager(bus core.EventBus, coreCtx ...core.Context) *Manager {
	var cCtx core.Context
	if len(coreCtx) > 0 && coreCtx[0] != nil {
		cCtx = coreCtx[0]
	} else {
		cCtx = core.NewContext(context.Background())
		if bus != nil {
			cCtx.SetService(core.ServiceEventBus, bus)
		}
	}

	m := &Manager{
		bus:            bus,
		coreCtx:        cCtx,
		activePlugins:  make(map[string]bool),
		pluginStates:   make(map[string]PluginState),
		pluginErrors:   make(map[string]string),
		pluginContexts: make(map[string]PluginContext),
		customSubs:     make(map[string][]customSubRef),
	}
	// 注册内建插件
	m.builtInPlugins = append(m.builtInPlugins,
		&MetadataMappingPlugin{},
		DefaultGeekNamingPlugin,
		&YucSchedulePlugin{},
		&DataMigratePlugin{},
		DefaultExtendedNotifiersPlugin,
		NewNFOScraperPlugin(),
	)
	return m
}

// SetCoreContext 动态绑定核心上下文
func (m *Manager) SetCoreContext(ctx core.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.coreCtx = ctx
}

// Load 从数据库读取插件配置与启停状态，并按 DSH 模式重建全部内建与自定义插件上下文与订阅
func (m *Manager) Load() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 取消旧的在途 Webhook 请求并重建上下文
	if m.webhookCancel != nil {
		m.webhookCancel()
	}
	m.webhookCtx, m.webhookCancel = context.WithCancel(context.Background())

	// 清理旧的自定义插件事件订阅
	if m.bus != nil {
		for _, subs := range m.customSubs {
			for _, s := range subs {
				m.bus.Unsubscribe(s.evType, s.subID)
			}
		}
	}
	m.customSubs = make(map[string][]customSubRef)

	// 读取插件启用状态与自定义插件清单（保护 database.DB 为 nil 的场景）
	if database.DB != nil {
		var statusSetting database.Setting
		if err := database.DB.Where("key = ?", "plugin_status").First(&statusSetting).Error; err == nil && statusSetting.Value != "" {
			loaded := make(map[string]bool)
			if err := json.Unmarshal([]byte(statusSetting.Value), &loaded); err != nil {
				log.Printf("⚠️ [插件] 解析 plugin_status 配置失败: %v", err)
			} else {
				m.activePlugins = loaded
			}
		}

		var customSetting database.Setting
		if err := database.DB.Where("key = ?", "custom_plugins").First(&customSetting).Error; err == nil {
			if customSetting.Value == "" {
				m.customPlugins = nil
			} else {
				var loadedCustom []PluginInfo
				if err := json.Unmarshal([]byte(customSetting.Value), &loadedCustom); err != nil {
					log.Printf("⚠️ [插件] 解析 custom_plugins 配置失败: %v", err)
				} else {
					m.customPlugins = loadedCustom
				}
			}
		}
	}

	// 管理内建插件生命周期（严格恐慌隔离与无感启停）
	for _, p := range m.builtInPlugins {
		info := p.GetInfo()
		enabled, exists := m.activePlugins[info.ID]
		if !exists {
			enabled = !isDefaultDisabled(info.ID)
		}

		if enabled {
			log.Printf("🔌 [插件] 启动内建插件: %s (%s)", info.Name, info.Version)
			m.pluginStates[info.ID] = StateLoading

			// 销毁旧上下文（若存在）
			if oldCtx, ok := m.pluginContexts[info.ID]; ok && oldCtx != nil {
				oldCtx.Dispose()
			}
			pctx := NewPluginContext(info.ID, m.bus, m.coreCtx)
			m.pluginContexts[info.ID] = pctx

			func() {
				defer func() {
					if r := recover(); r != nil {
						errMsg := fmt.Sprintf("crash during plugin init: %v", r)
						log.Printf("❌ [插件 %s] 启动捕获 panic: %v", info.ID, r)
						m.pluginStates[info.ID] = StateError
						m.pluginErrors[info.ID] = errMsg
						pctx.Dispose()
						delete(m.pluginContexts, info.ID)
					}
				}()
				if err := p.Init(m.bus, pctx); err != nil {
					log.Printf("❌ [插件] 启动失败 [%s]: %v", info.ID, err)
					m.pluginStates[info.ID] = StateError
					m.pluginErrors[info.ID] = err.Error()
					pctx.Dispose()
					delete(m.pluginContexts, info.ID)
				} else {
					m.pluginStates[info.ID] = StateActive
					delete(m.pluginErrors, info.ID)
				}
			}()
		} else {
			m.pluginStates[info.ID] = StateDisposing
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("❌ [插件 %s] 停用捕获 panic: %v", info.ID, r)
						m.pluginStates[info.ID] = StateError
						m.pluginErrors[info.ID] = fmt.Sprintf("crash during plugin stop: %v", r)
					}
				}()
				if err := p.Stop(m.bus); err != nil {
					log.Printf("❌ [插件] 停用失败 [%s]: %v", info.ID, err)
				}
			}()

			if oldCtx, ok := m.pluginContexts[info.ID]; ok && oldCtx != nil {
				oldCtx.Dispose()
				delete(m.pluginContexts, info.ID)
			}
			if m.pluginStates[info.ID] != StateError {
				m.pluginStates[info.ID] = StateInactive
				delete(m.pluginErrors, info.ID)
			}
		}
	}

	// 启动自定义 Webhook 插件
	for _, cp := range m.customPlugins {
		enabled, exists := m.activePlugins[cp.ID]
		if !exists {
			enabled = cp.Enabled
		}
		if enabled && cp.Type == "webhook" && cp.URL != "" {
			m.startWebhookPlugin(cp)
		} else {
			if oldCtx, ok := m.pluginContexts[cp.ID]; ok && oldCtx != nil {
				oldCtx.Dispose()
				delete(m.pluginContexts, cp.ID)
			}
			m.pluginStates[cp.ID] = StateInactive
			delete(m.pluginErrors, cp.ID)
		}
	}
}

// StopAll 优雅停用全部内建插件与自定义 Webhook 订阅，并等待在途协程退出
func (m *Manager) StopAll() {
	m.mu.Lock()
	if m.webhookCancel != nil {
		m.webhookCancel()
		m.webhookCancel = nil
	}
	m.webhookCtx = nil

	if m.bus != nil {
		for _, subs := range m.customSubs {
			for _, s := range subs {
				m.bus.Unsubscribe(s.evType, s.subID)
			}
		}
	}
	m.customSubs = make(map[string][]customSubRef)

	plugins := make([]BuiltInPlugin, len(m.builtInPlugins))
	copy(plugins, m.builtInPlugins)

	contextsToDispose := make([]PluginContext, 0, len(m.pluginContexts))
	for id, pctx := range m.pluginContexts {
		if pctx != nil {
			contextsToDispose = append(contextsToDispose, pctx)
		}
		m.pluginStates[id] = StateInactive
	}
	m.pluginContexts = make(map[string]PluginContext)
	bus := m.bus
	m.mu.Unlock()

	// 在锁外逐一停用内建插件，避免插件 Stop 等待后台任务时发生锁竞争
	for _, p := range plugins {
		info := p.GetInfo()
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("❌ [插件 %s] 停用捕获 panic: %v", info.ID, r)
				}
			}()
			if err := p.Stop(bus); err != nil {
				log.Printf("❌ [插件] 停用失败 [%s]: %v", info.ID, err)
			}
		}()
	}

	// 统一 Dispose 全部活跃上下文，释放 LIFO 清理回调
	for _, pctx := range contextsToDispose {
		pctx.Dispose()
	}

	// 等待所有在途 Webhook 协程彻底结束
	m.webhookWg.Wait()
}

func (m *Manager) startWebhookPlugin(p PluginInfo) {
	if m.bus == nil {
		return
	}

	// 清理旧上下文
	if oldCtx, ok := m.pluginContexts[p.ID]; ok && oldCtx != nil {
		oldCtx.Dispose()
		delete(m.pluginContexts, p.ID)
	}

	if _, err := validateExternalHTTPURL(p.URL, m.allowPrivateWebhookForTest); err != nil {
		log.Printf("⚠️ [插件 %s] Webhook URL 校验未通过，跳过挂载: %v", p.Name, err)
		m.pluginStates[p.ID] = StateError
		m.pluginErrors[p.ID] = fmt.Sprintf("非法的 Webhook URL: %v", err)
		return
	}

	pctx := NewPluginContext(p.ID, m.bus, m.coreCtx)
	m.pluginContexts[p.ID] = pctx
	m.pluginStates[p.ID] = StateActive
	delete(m.pluginErrors, p.ID)

	seenEvents := make(map[string]struct{}, len(p.Events))
	for _, evType := range p.Events {
		et := strings.TrimSpace(evType)
		if et == "" {
			continue
		}
		if _, dup := seenEvents[et]; dup {
			continue
		}
		seenEvents[et] = struct{}{}

		targetURL := p.URL
		secret := p.Secret
		pluginID := p.ID
		pluginName := p.Name
		allowPrivate := m.allowPrivateWebhookForTest

		subID := pctx.Subscribe(et, func(ev core.Event) {
			// 在读锁下检查上下文并登记 WaitGroup，防止与 StopAll 的 Wait 产生竞态
			m.mu.RLock()
			baseCtx := m.webhookCtx
			if baseCtx == nil || baseCtx.Err() != nil {
				m.mu.RUnlock()
				return
			}
			m.webhookWg.Add(1)
			m.mu.RUnlock()

			pctx.Go(func(workerCtx context.Context) {
				defer m.webhookWg.Done()
				defer func() {
					if r := recover(); r != nil {
						log.Printf("⚠️ [插件 %s] Webhook 发送异常崩溃捕获: %v", pluginName, r)
					}
				}()

				payloadData, err := json.Marshal(map[string]interface{}{
					"event":     ev.Type,
					"timestamp": ev.Time.Unix(),
					"payload":   ev.Payload,
				})
				if err != nil {
					log.Printf("⚠️ [插件 %s] Webhook 负载序列化失败: %v", pluginName, err)
					return
				}

				reqCtx, cancel := context.WithTimeout(workerCtx, 10*time.Second)
				defer cancel()

				req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, targetURL, bytes.NewReader(payloadData))
				if err != nil {
					log.Printf("⚠️ [插件 %s] 构建 Webhook 请求失败: %v", pluginName, err)
					return
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "Ani-Go-Webhook/"+pluginID)
				if secret != "" {
					req.Header.Set("X-AniGo-Secret", secret)
				}

				client := newSSRFSafeHTTPClient(10*time.Second, allowPrivate)
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("⚠️ [插件 %s] Webhook 发送失败: %v", pluginName, err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode >= http.StatusBadRequest {
					snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
					log.Printf("⚠️ [插件 %s] Webhook 返回异常状态码 %d: %s", pluginName, resp.StatusCode, strings.TrimSpace(string(snippet)))
					return
				}
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			})
		})
		m.customSubs[p.ID] = append(m.customSubs[p.ID], customSubRef{evType: et, subID: subID})
	}
	log.Printf("🔌 [插件] 已加载自定义 Webhook: %s -> %s (事件: %v)", p.Name, p.URL, p.Events)
}

func isDefaultDisabled(id string) bool {
	return id == "yuc_schedule" || id == "nfo_scraper"
}

func clonePluginInfo(p PluginInfo) PluginInfo {
	cp := p
	if p.Events != nil {
		cp.Events = make([]string, len(p.Events))
		copy(cp.Events, p.Events)
	}
	return cp
}

// GetPluginList 返回全部内建插件与自定义插件的当前状态快照（含详细健康状态与最近错误）
func (m *Manager) GetPluginList() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]PluginInfo, 0, len(m.builtInPlugins)+len(m.customPlugins))
	for _, p := range m.builtInPlugins {
		info := clonePluginInfo(p.GetInfo())
		enabled, exists := m.activePlugins[info.ID]
		if !exists {
			enabled = !isDefaultDisabled(info.ID)
		}
		info.Enabled = enabled

		st, hasStatus := m.pluginStates[info.ID]
		if !hasStatus {
			if enabled {
				st = StateActive
			} else {
				st = StateInactive
			}
		}
		info.Status = st
		info.Error = m.pluginErrors[info.ID]

		list = append(list, info)
	}
	for _, cp := range m.customPlugins {
		item := clonePluginInfo(cp)
		enabled, exists := m.activePlugins[item.ID]
		if !exists {
			enabled = item.Enabled
		}
		item.Enabled = enabled

		st, hasStatus := m.pluginStates[item.ID]
		if !hasStatus {
			if enabled {
				st = StateActive
			} else {
				st = StateInactive
			}
		}
		item.Status = st
		item.Error = m.pluginErrors[item.ID]

		list = append(list, item)
	}
	return list
}

// GetPluginStatus 查询单个插件的当前运行状态与错误信息
func (m *Manager) GetPluginStatus(id string) (PluginState, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st, exists := m.pluginStates[id]
	if !exists {
		enabled, ok := m.activePlugins[id]
		if !ok {
			enabled = !isDefaultDisabled(id)
		}
		if enabled {
			st = StateActive
		} else {
			st = StateInactive
		}
	}
	return st, m.pluginErrors[id]
}

// GetPluginContext 获取指定已激活插件的 PluginContext 实例
func (m *Manager) GetPluginContext(id string) PluginContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pluginContexts[id]
}

// IsPluginEnabled 查询指定插件当前是否处于启用状态
func (m *Manager) IsPluginEnabled(id string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	enabled, exists := m.activePlugins[id]
	if !exists {
		return !isDefaultDisabled(id)
	}
	return enabled
}

// GetPlugins 返回插件列表（兼容旧接口）
func (m *Manager) GetPlugins() interface{} {
	return m.GetPluginList()
}

// Reload 重新加载全部插件状态与订阅
func (m *Manager) Reload() {
	m.Load()
}

// GetBus 获取管理器绑定的事件总线
func (m *Manager) GetBus() core.EventBus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bus
}

// TogglePlugin 切换指定插件的启用状态并持久化到数据库
func (m *Manager) TogglePlugin(id string, enabled bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("插件 ID 不能为空")
	}

	m.mu.Lock()
	prevVal, hadPrev := m.activePlugins[id]
	m.activePlugins[id] = enabled
	data, err := json.Marshal(m.activePlugins)
	if err != nil {
		if hadPrev {
			m.activePlugins[id] = prevVal
		} else {
			delete(m.activePlugins, id)
		}
		m.mu.Unlock()
		return fmt.Errorf("序列化插件状态失败: %w", err)
	}
	if database.DB != nil {
		if err := database.DB.Save(&database.Setting{Key: "plugin_status", Value: string(data)}).Error; err != nil {
			if hadPrev {
				m.activePlugins[id] = prevVal
			} else {
				delete(m.activePlugins, id)
			}
			m.mu.Unlock()
			return fmt.Errorf("保存插件状态失败: %w", err)
		}
	}
	m.mu.Unlock()

	m.Load()
	return nil
}

// List 获取插件列表
func (m *Manager) List() []PluginInfo {
	return m.GetPluginList()
}

// Enable 启用指定插件
func (m *Manager) Enable(id string) error {
	return m.TogglePlugin(id, true)
}

// Disable 停用指定插件
func (m *Manager) Disable(id string) error {
	return m.TogglePlugin(id, false)
}

// AddOrUpdatePlugin 新增或更新自定义插件配置，并立即生效
func (m *Manager) AddOrUpdatePlugin(p PluginInfo) error {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.URL = strings.TrimSpace(p.URL)
	if p.ID == "" {
		p.ID = fmt.Sprintf("custom-%d", time.Now().UnixNano())
	}
	if p.Name == "" {
		return fmt.Errorf("插件名称不能为空")
	}
	p.IsBuiltIn = false
	if p.Type == "" {
		p.Type = "webhook"
	}
	if p.Type == "builtin" {
		return fmt.Errorf("不允许注册或覆盖内建类型插件")
	}
	if p.Type == "webhook" {
		if p.URL == "" {
			return fmt.Errorf("Webhook URL 不能为空")
		}
		if _, err := validateExternalHTTPURL(p.URL, m.allowPrivateWebhookForTest); err != nil {
			return fmt.Errorf("非法的 Webhook URL: %w", err)
		}
	}
	if p.Icon == "" {
		p.Icon = "Sparkles"
	}
	p = clonePluginInfo(p)

	m.mu.Lock()
	prevList := make([]PluginInfo, len(m.customPlugins))
	copy(prevList, m.customPlugins)

	updated := false
	for i, cp := range m.customPlugins {
		if cp.ID == p.ID {
			m.customPlugins[i] = p
			updated = true
			break
		}
	}
	if !updated {
		p.Enabled = true
		m.customPlugins = append(m.customPlugins, p)
	}
	data, err := json.Marshal(m.customPlugins)
	if err != nil {
		m.customPlugins = prevList
		m.mu.Unlock()
		return fmt.Errorf("序列化自定义插件失败: %w", err)
	}
	if database.DB != nil {
		if err := database.DB.Save(&database.Setting{Key: "custom_plugins", Value: string(data)}).Error; err != nil {
			m.customPlugins = prevList
			m.mu.Unlock()
			return fmt.Errorf("保存自定义插件失败: %w", err)
		}
	}
	m.mu.Unlock()

	m.Load()
	return nil
}

// DeletePlugin 删除自定义插件并解除对应事件订阅
func (m *Manager) DeletePlugin(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("插件 ID 不能为空")
	}

	m.mu.Lock()
	prevList := make([]PluginInfo, len(m.customPlugins))
	copy(prevList, m.customPlugins)

	newList := make([]PluginInfo, 0, len(m.customPlugins))
	for _, cp := range m.customPlugins {
		if cp.ID != id {
			newList = append(newList, cp)
		}
	}
	m.customPlugins = newList
	if oldCtx, ok := m.pluginContexts[id]; ok && oldCtx != nil {
		oldCtx.Dispose()
		delete(m.pluginContexts, id)
	}
	delete(m.pluginStates, id)
	delete(m.pluginErrors, id)

	data, err := json.Marshal(m.customPlugins)
	if err != nil {
		m.customPlugins = prevList
		m.mu.Unlock()
		return fmt.Errorf("序列化自定义插件失败: %w", err)
	}
	if database.DB != nil {
		if err := database.DB.Save(&database.Setting{Key: "custom_plugins", Value: string(data)}).Error; err != nil {
			m.customPlugins = prevList
			m.mu.Unlock()
			return fmt.Errorf("保存自定义插件失败: %w", err)
		}
	}
	m.mu.Unlock()

	m.Load()
	return nil
}

// validateExternalHTTPURL 校验外部 HTTP(S) URL 合法性并拦截常见 SSRF 目标
func validateExternalHTTPURL(rawURL string, allowPrivate bool) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("URL 不能为空")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("URL 解析失败: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("不支持的协议 %q，仅允许 http/https", u.Scheme)
	}
	if u.User != nil {
		return nil, fmt.Errorf("URL 不允许包含认证凭据")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return nil, fmt.Errorf("URL 缺少有效主机名")
	}
	if allowPrivate {
		return u, nil
	}

	lowerHost := strings.ToLower(strings.TrimSuffix(host, "."))
	if lowerHost == "localhost" ||
		lowerHost == "localhost.localdomain" ||
		lowerHost == "ip6-localhost" ||
		strings.HasSuffix(lowerHost, ".localhost") ||
		strings.HasSuffix(lowerHost, ".local") ||
		strings.HasSuffix(lowerHost, ".internal") {
		return nil, fmt.Errorf("禁止访问本地或内网域名: %s", host)
	}

	if ip := net.ParseIP(host); ip != nil && isDisallowedIP(ip) {
		return nil, fmt.Errorf("禁止访问内网或保留 IP 地址: %s", ip.String())
	}
	return u, nil
}

// isDisallowedIP 判断 IP 是否属于回环、内网、链路本地、云元数据或保留网段
func isDisallowedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
		// 0.0.0.0/8 ("this" network)
		if ip[0] == 0 {
			return true
		}
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// ssrfSafeTransport 共享的安全 HTTP 传输层（阻断内网/回环/元数据 IP，避免每次创建新连接池）
var ssrfSafeTransport = &http.Transport{
	Proxy: nil, // 禁止经由本地代理绕过 IP 校验
	DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("主机 %s 未解析到任何 IP", host)
		}
		var safeIP net.IP
		for _, ipAddr := range ips {
			if isDisallowedIP(ipAddr.IP) {
				return nil, fmt.Errorf("拦截不安全的目标 IP: %s (%s)", ipAddr.IP.String(), host)
			}
			if safeIP == nil {
				safeIP = ipAddr.IP
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(safeIP.String(), port))
	},
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          20,
	MaxIdleConnsPerHost:   5,
	IdleConnTimeout:       30 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// newSSRFSafeHTTPClient 创建带超时、重定向拦截及 DNS 解析 IP 校验的 HTTP 客户端
func newSSRFSafeHTTPClient(timeout time.Duration, allowPrivate bool) *http.Client {
	if allowPrivate {
		return &http.Client{
			Timeout:   timeout,
			Transport: httpx.Default.Transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("重定向次数过多")
				}
				_, err := validateExternalHTTPURL(req.URL.String(), true)
				return err
			},
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: ssrfSafeTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			_, err := validateExternalHTTPURL(req.URL.String(), false)
			return err
		},
	}
}
