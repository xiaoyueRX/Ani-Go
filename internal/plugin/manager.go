package plugin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

// Manager 插件生命周期管理器，统一负责内建插件与自定义 Webhook 插件的加载、启停、健康追踪与热重载
type Manager struct {
	bus                        core.EventBus
	coreCtx                    core.ServiceRegistry // 核心系统万能上下文
	activationOrder            []string
	bindings                   map[string]map[core.ServiceType]uint64
	stopDone                   chan struct{}
	refreshQueued              atomic.Bool
	serial                     sync.Mutex // 串行化配置和生命周期变更，读取状态无需等待插件回调。
	restoring                  bool
	configurationPending       bool // 已提交的恢复等待可信配置，期间不按旧配置启动。
	draining                   map[string]*pluginContextImpl
	observedBus                map[string]bool
	shutdown                   chan struct{}
	mu                         sync.RWMutex
	builtInPlugins             []BuiltInPlugin
	customPlugins              []PluginInfo
	activePlugins              map[string]bool          // id -> enabled
	pluginStates               map[string]PluginState   // id -> PluginState (active/inactive/error/loading/disposing)
	pluginErrors               map[string]string        // id -> 最近错误信息
	pluginContexts             map[string]PluginContext // id -> PluginContext
	runningWebhooks            map[string]PluginInfo
	allowPrivateWebhookForTest bool // 仅限单元测试使用，允许连接回环/私网地址
}

// NewManager 创建插件管理器实例并注册全部内建插件，支持注入可选的核心上下文
func NewManager(bus core.EventBus, coreCtx ...core.Context) *Manager {
	var cCtx core.Context
	if len(coreCtx) > 0 && coreCtx[0] != nil {
		cCtx = coreCtx[0]
	} else {
		cCtx = core.NewContext(context.Background())
	}
	registry := core.TrackServices(cCtx)
	bus = rootEventBus(registry, bus)

	m := &Manager{
		bus:             bus,
		coreCtx:         registry,
		bindings:        make(map[string]map[core.ServiceType]uint64),
		draining:        make(map[string]*pluginContextImpl),
		observedBus:     make(map[string]bool),
		shutdown:        make(chan struct{}),
		pluginStates:    make(map[string]PluginState),
		pluginErrors:    make(map[string]string),
		pluginContexts:  make(map[string]PluginContext),
		runningWebhooks: make(map[string]PluginInfo),
	}
	// 注册内建插件
	m.builtInPlugins = append(m.builtInPlugins,
		&MetadataMappingPlugin{},
		DefaultGeekNamingPlugin,
		&YucSchedulePlugin{},
		&DataMigratePlugin{},
		DefaultExtendedNotifiersPlugin,
		NewNFOScraperPlugin(),
		&Cloud115Plugin{},
	)
	return m
}

// SetCoreContext 动态绑定核心上下文
func (m *Manager) SetCoreContext(ctx core.Context) {
	m.lockChange()
	defer m.unlockChange()
	if ctx == nil || m.stopDone != nil {
		return
	}
	m.retireInstancesLocked()
	fallbackBus := registeredEventBus(m.coreCtx)
	m.coreCtx = core.TrackServices(ctx)
	m.bus = rootEventBus(m.coreCtx, fallbackBus)
	if m.activePlugins != nil {
		m.applyRuntimeLocked()
	}
}

// Load 从数据库读取插件配置与启停状态，并使运行中的插件与配置保持一致
func (m *Manager) Load() error {
	m.lockChange()
	defer m.unlockChange()
	if err := m.updateConfigurationLocked(0, nil); err != nil {
		log.Printf("⚠️ [插件] 加载配置失败: %v", err)
		return err
	}
	return nil
}

func (m *Manager) retireInstancesLocked() {
	plugins := m.builtinsByID()
	for _, id := range slices.Backward(slices.Clone(m.activationOrder)) {
		m.stopActiveLocked(id, plugins[id])
	}
	m.runningWebhooks = make(map[string]PluginInfo)
}

func (m *Manager) applyRuntimeLocked() {
	if m.configurationPending {
		return
	}
	// 即使清单完全相同，恢复前的在途动作也不可继续使用恢复后的授权。
	if m.restoring {
		m.retireInstancesLocked()
	}
	m.bus = registeredEventBus(m.coreCtx)
	m.reconcileBuiltinsLocked()
	m.bus = registeredEventBus(m.coreCtx)

	// 只替换配置发生变化的 Webhook，保留其他插件的在途请求与订阅。
	nextWebhooks := make(map[string]PluginInfo)
	for _, cp := range m.customPlugins {
		if m.draining[cp.ID] != nil {
			m.pluginStates[cp.ID] = StateDisposing
			continue
		}
		enabled := m.enabledLocked(cp.ID, cp.Enabled)
		if enabled && cp.Type == "webhook" && cp.URL != "" {
			deps := webhookDependencies(cp)
			if len(m.missingServices(deps)) != 0 {
				m.pluginStates[cp.ID] = StateSuspended
				delete(m.pluginErrors, cp.ID)
				continue
			}
			nextWebhooks[cp.ID] = cp
			prev, running := m.runningWebhooks[cp.ID]
			if running && m.pluginStates[cp.ID] == StateActive && m.pluginContexts[cp.ID] != nil && m.pluginContexts[cp.ID].Err() == nil && m.bindingsCurrent(cp.ID, deps) &&
				prev.URL == cp.URL && prev.Name == cp.Name && prev.Secret == cp.Secret && slices.Equal(prev.Events, cp.Events) && reflect.DeepEqual(prev.Panel, cp.Panel) {
				continue
			}
			m.forgetActivation(cp.ID)
			m.startWebhookPlugin(cp)
			if m.pluginStates[cp.ID] == StateActive {
				m.recordActivation(cp.ID, deps)
			}
		} else {
			m.pluginStates[cp.ID] = StateInactive
			delete(m.pluginErrors, cp.ID)
		}
	}
	for id := range m.runningWebhooks {
		if _, keep := nextWebhooks[id]; !keep {
			m.forgetActivation(id)
			if oldCtx := m.pluginContexts[id]; oldCtx != nil {
				m.disposeScopeLocked(id, oldCtx)
				delete(m.pluginContexts, id)
			}
		}
	}
	known := make(map[string]bool, len(m.customPlugins)+len(m.builtInPlugins))
	for _, p := range m.customPlugins {
		known[p.ID] = true
	}
	for _, p := range m.builtInPlugins {
		known[p.GetInfo().ID] = true
	}
	for id := range m.pluginStates {
		if !known[id] {
			delete(m.pluginStates, id)
			delete(m.pluginErrors, id)
		}
	}
	m.runningWebhooks = nextWebhooks
}

// StopAll 优雅停用全部内建插件与自定义 Webhook 订阅，并等待在途协程退出
func (m *Manager) StopAll() {
	m.lockChange()
	if done := m.stopDone; done != nil {
		m.unlockChange()
		<-done
		return
	}
	m.stopDone = make(chan struct{})
	close(m.shutdown)
	done := m.stopDone
	bus := m.bus
	type active struct {
		plugin BuiltInPlugin
		ctx    PluginContext
	}
	var shutdown []active
	plugins := m.builtinsByID()
	for _, id := range slices.Backward(m.activationOrder) {
		shutdown = append(shutdown, active{plugins[id], m.pluginContexts[id]})
	}
	for id := range m.pluginStates {
		m.pluginStates[id] = StateInactive
	}
	clear(m.pluginContexts)
	clear(m.runningWebhooks)
	clear(m.bindings)
	m.activationOrder = nil
	m.unlockChange()
	defer close(done)
	for _, item := range shutdown {
		pluginBus := bus
		if c, ok := item.ctx.(*pluginContextImpl); ok {
			c.cancelForStop()
			pluginBus = c.EventBus()
		}
		if item.plugin != nil {
			stopBuiltin(item.plugin, pluginBus)
		}
		if item.ctx != nil {
			item.ctx.Dispose()
			if c, ok := item.ctx.(*pluginContextImpl); ok && item.plugin == nil {
				c.wg.Wait()
			}
		}
	}
}

func (m *Manager) startWebhookPlugin(p PluginInfo) {
	// 清理旧上下文
	if oldCtx := m.pluginContexts[p.ID]; oldCtx != nil {
		m.pluginStates[p.ID] = StateDisposing
		m.disposeScopeLocked(p.ID, oldCtx)
		delete(m.pluginContexts, p.ID)
		if m.draining[p.ID] != nil {
			return
		}
	}

	if _, err := validateExternalHTTPURL(p.URL, m.allowPrivateWebhookForTest); err != nil {
		log.Printf("⚠️ [插件 %s] Webhook URL 校验未通过，跳过挂载", p.Name)
		m.pluginStates[p.ID] = StateError
		m.pluginErrors[p.ID] = "Webhook URL 无效"
		return
	}

	if err := validateWebhookPanel(p, m.allowPrivateWebhookForTest); err != nil {
		m.pluginStates[p.ID] = StateError
		m.pluginErrors[p.ID] = err.Error()
		return
	}
	pctx := m.newScope(p.ID)
	registerWebhookPanel(pctx, p, m.allowPrivateWebhookForTest)
	m.pluginContexts[p.ID] = pctx
	m.pluginStates[p.ID] = StateActive
	delete(m.pluginErrors, p.ID)

	client := webhookDeliveryClient(m.allowPrivateWebhookForTest, p.Secret != "")
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

		busEvent := et
		switch et {
		case "episode.missing":
			busEvent = core.EventSupplementCompleted
		case "error":
			busEvent = core.EventDownloadFailed
		}
		pctx.Subscribe(busEvent, func(ev core.Event) {
			// 旧版“缺集”订阅只接收有新增的扫描，回调负载保留配置中的事件名。
			if et == "episode.missing" && extractPayloadInt(ev.Payload["new_count"]) == 0 {
				return
			}
			// 总线已为该订阅独立排队，网络请求留在受管回调内，停用时由作用域取消。

			payloadData, err := json.Marshal(map[string]interface{}{
				"event":     et,
				"timestamp": ev.Time.Unix(),
				"payload":   ev.Payload,
			})
			if err != nil {
				log.Printf("⚠️ [插件 %s] Webhook 负载序列化失败: %v", pluginName, err)
				return
			}

			reqCtx, cancel := context.WithTimeout(pctx.Context(), 10*time.Second)
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

			resp, err := client.Do(req)
			if err != nil {
				if secret == "" {
					log.Printf("⚠️ [插件 %s] Webhook 发送失败: %v", pluginName, err)
				} else {
					log.Printf("⚠️ [插件 %s] 已授权的 Webhook 请求失败", pluginName)
				}
				return
			}
			defer resp.Body.Close()

			if secret != "" && resp.StatusCode >= http.StatusMultipleChoices {
				log.Printf("⚠️ [插件 %s] 已授权的 Webhook 返回状态码 %d", pluginName, resp.StatusCode)
				return
			}
			if resp.StatusCode >= http.StatusBadRequest {
				snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				log.Printf("⚠️ [插件 %s] Webhook 返回异常状态码 %d: %s", pluginName, resp.StatusCode, strings.TrimSpace(string(snippet)))
				return
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		})
	}
	log.Printf("🔌 [插件] 已加载自定义 Webhook: %s (事件: %v)", p.Name, p.Events)
}

// 共享密钥只授权清单中的接收地址，其他地址需要单独配置。
func webhookDeliveryClient(allowPrivate, hasSecret bool) *http.Client {
	client := newSSRFSafeHTTPClient(10*time.Second, allowPrivate)
	if hasSecret {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return client
}

func isDefaultDisabled(id string) bool {
	return id == "yuc_schedule" || id == "nfo_scraper" || id == Cloud115ID
}

func clonePluginInfo(p PluginInfo) PluginInfo {
	cp := p
	cp.Dependencies = slices.Clone(p.Dependencies)
	cp.MissingDependencies = slices.Clone(p.MissingDependencies)
	cp.Panel = p.Panel.clone()
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
		info.Dependencies = m.builtinDependencies(p)
		info.MissingDependencies = nil
		if m.pluginStates[info.ID] == StateSuspended {
			info.MissingDependencies = m.missingServices(info.Dependencies)
		}
		info.Secret = ""
		info.IsBuiltIn = true
		info.Origin = OriginCommunity
		if officialPluginIDs[info.ID] {
			info.Origin = OriginOfficial
		}
		enabled := m.enabledLocked(info.ID, !isDefaultDisabled(info.ID))
		info.Enabled = enabled

		st, hasStatus := m.pluginStates[info.ID]
		if !hasStatus {
			st = StateInactive
		}
		info.Status = st
		info.Error = m.pluginErrors[info.ID]

		list = append(list, info)
	}
	for _, cp := range m.customPlugins {
		list = append(list, m.customSnapshotLocked(cp))
	}
	return list
}

func (m *Manager) customSnapshotLocked(p PluginInfo) PluginInfo {
	info := clonePluginInfo(p)
	info.Secret = ""
	info.Enabled = m.enabledLocked(info.ID, info.Enabled)
	info.Status = m.pluginStates[info.ID]
	if info.Status == "" {
		info.Status = StateInactive
	}
	info.Error = m.pluginErrors[info.ID]
	if info.Type == "webhook" {
		info.Dependencies = webhookDependencies(p)
	}
	info.MissingDependencies = nil
	if info.Status == StateSuspended {
		info.MissingDependencies = m.missingServices(info.Dependencies)
	}
	return info
}

// GetPluginStatus 查询单个插件的当前运行状态与错误信息
func (m *Manager) GetPluginStatus(id string) (PluginState, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if state, ok := m.pluginStates[id]; ok {
		return state, m.pluginErrors[id]
	}
	return StateInactive, ""

}

// GetPluginContext 获取指定已激活插件的 PluginContext 实例
func (m *Manager) GetPluginContext(id string) PluginContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	scope := m.pluginContexts[id]
	if m.restoring || m.stopDone != nil || m.pluginStates[id] != StateActive || scope == nil || scope.Err() != nil {
		return nil
	}
	return scope
}

// IsPluginActive 检查插件是否已成功启动，且当前实例仍可处理业务请求。
func (m *Manager) IsPluginActive(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	scope := m.pluginContexts[id]
	return !m.restoring && m.stopDone == nil && m.pluginStates[id] == StateActive && scope != nil && scope.Context().Err() == nil
}

// IsPluginEnabled 查询指定插件当前是否处于启用状态
func (m *Manager) IsPluginEnabled(id string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopDone != nil {
		return false
	}
	for _, p := range m.builtInPlugins {
		if p.GetInfo().ID == id {
			return m.enabledLocked(id, !isDefaultDisabled(id))
		}
	}
	for _, p := range m.customPlugins {
		if p.ID == id {
			return m.enabledLocked(id, p.Enabled)
		}
	}
	return false
}

func (m *Manager) enabledLocked(id string, defaultValue bool) bool {
	if m.activePlugins == nil {
		return false
	}
	enabled, exists := m.activePlugins[id]
	if !exists {
		return defaultValue
	}
	return enabled
}

// GetPlugins 返回插件列表（兼容旧接口）
func (m *Manager) GetPlugins() interface{} {
	return m.GetPluginList()
}

// Reload 重新加载全部插件状态与订阅
func (m *Manager) Reload() error {
	return m.Load()
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

	m.lockChange()
	defer m.unlockChange()
	return m.updateConfigurationLocked(statusChanged, func(config *pluginConfiguration) error {
		_, builtin := m.builtinsByID()[id]
		if !builtin && !slices.ContainsFunc(config.custom, func(p PluginInfo) bool { return p.ID == id }) {
			return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
		}
		config.status[id] = enabled
		return nil
	})
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
	return m.SaveCustomPlugin(p, &p.Secret)
}

// SaveCustomPlugin 接收可选的 Webhook 密钥。同一目标可省略密钥，显式空值会清除密钥。
func (m *Manager) SaveCustomPlugin(p PluginInfo, secret *string) error {
	_, err := m.SaveCustomPluginInfo(p, secret)
	return err
}

// SaveCustomPluginInfo 返回已提交并应用的清单和运行状态，省略密钥。
func (m *Manager) SaveCustomPluginInfo(p PluginInfo, secret *string) (PluginInfo, error) {
	p.Secret = ""
	if secret != nil {
		p.Secret = *secret
	}
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	if p.ID == "." || p.ID == ".." {
		return PluginInfo{}, fmt.Errorf("%w：无效的插件 ID", ErrInvalidPluginManifest)
	}
	p.URL = strings.TrimSpace(p.URL)
	if p.ID == "" {
		p.ID = "custom-" + rand.Text()
	}
	if p.Name == "" {
		return PluginInfo{}, fmt.Errorf("%w：插件名称不能为空", ErrInvalidPluginManifest)
	}
	if m.reservedPluginID(p.ID) {
		return PluginInfo{}, fmt.Errorf("%w: %s", ErrReservedPluginID, p.ID)
	}
	p.IsBuiltIn = false
	p.Origin = OriginCommunity
	if p.Type == "" {
		p.Type = "webhook"
	}
	if p.Type != "webhook" {
		return PluginInfo{}, fmt.Errorf("%w：仅支持 webhook 类型的自定义插件", ErrInvalidPluginManifest)
	}
	if p.URL == "" {
		return PluginInfo{}, fmt.Errorf("%w：Webhook URL 不能为空", ErrInvalidPluginManifest)
	}
	if _, err := validateExternalHTTPURL(p.URL, m.allowPrivateWebhookForTest); err != nil {
		return PluginInfo{}, fmt.Errorf("%w：Webhook URL 无效", ErrInvalidPluginManifest)
	}
	if err := validateWebhookPanel(p, m.allowPrivateWebhookForTest); err != nil {
		return PluginInfo{}, fmt.Errorf("%w：%v", ErrInvalidPluginManifest, err)
	}
	if p.Icon == "" {
		p.Icon = "Sparkles"
	}
	p = clonePluginInfo(p)

	m.lockChange()
	defer m.unlockChange()
	err := m.updateConfigurationLocked(customChanged, func(config *pluginConfiguration) error {
		for i, cp := range config.custom {
			if cp.ID != p.ID {
				continue
			}
			p.Enabled = cp.Enabled
			if secret == nil && p.Type == "webhook" && cp.Type == "webhook" && p.URL == cp.URL {
				p.Secret = cp.Secret
			}
			if p.Author == "" {
				p.Author = cp.Author
			}
			if p.AuthorURL == "" && p.Author == cp.Author {
				p.AuthorURL = cp.AuthorURL
			}
			config.custom[i] = p
			return nil
		}
		p.Enabled = true
		config.custom = append(config.custom, p)
		return nil
	})
	if err != nil {
		return PluginInfo{}, err
	}
	return m.customSnapshotLocked(p), nil
}

// DeletePlugin 删除自定义插件并解除对应事件订阅
func (m *Manager) DeletePlugin(id string) error {
	id = strings.TrimSpace(id)
	if m.reservedPluginID(id) {
		return fmt.Errorf("%w: %s", ErrReservedPluginID, id)
	}
	if id == "" {
		return fmt.Errorf("插件 ID 不能为空")
	}

	m.lockChange()
	defer m.unlockChange()
	if err := m.updateConfigurationLocked(customChanged|statusChanged, func(config *pluginConfiguration) error {
		i := slices.IndexFunc(config.custom, func(p PluginInfo) bool { return p.ID == id })
		if i < 0 {
			return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
		}
		config.custom = slices.Delete(config.custom, i, i+1)
		delete(config.status, id)
		return nil
	}); err != nil {
		return err
	}
	delete(m.pluginStates, id)
	delete(m.pluginErrors, id)
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
