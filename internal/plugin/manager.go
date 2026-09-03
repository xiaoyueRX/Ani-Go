package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

type Manager struct {
	bus            core.EventBus
	mu             sync.RWMutex
	builtInPlugins []BuiltInPlugin
	customPlugins  []PluginInfo
	activePlugins  map[string]bool // id -> enabled
	customSubs     map[string][]customSubRef
}

func NewManager(bus core.EventBus) *Manager {
	m := &Manager{
		bus:           bus,
		activePlugins: make(map[string]bool),
		customSubs:    make(map[string][]customSubRef),
	}
	// 注册内建插件
	m.builtInPlugins = append(m.builtInPlugins, &MetadataMappingPlugin{}, &GeekNamingPlugin{})
	return m
}

func (m *Manager) Load() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清理之前的自定义插件事件订阅
	for _, subs := range m.customSubs {
		for _, s := range subs {
			m.bus.Unsubscribe(s.evType, s.subID)
		}
	}
	m.customSubs = make(map[string][]customSubRef)

	// 读取插件启用状态
	var statusSetting database.Setting
	if err := database.DB.Where("key = ?", "plugin_status").First(&statusSetting).Error; err == nil {
		json.Unmarshal([]byte(statusSetting.Value), &m.activePlugins)
	}

	// 读取自定义插件列表
	var customSetting database.Setting
	m.customPlugins = nil
	if err := database.DB.Where("key = ?", "custom_plugins").First(&customSetting).Error; err == nil && customSetting.Value != "" {
		json.Unmarshal([]byte(customSetting.Value), &m.customPlugins)
	}

	// 启动内建插件
	for _, p := range m.builtInPlugins {
		info := p.GetInfo()
		enabled, exists := m.activePlugins[info.ID]
		if !exists {
			enabled = true
		}

		if enabled {
			log.Printf("🔌 [插件] 启动内建插件: %s (%s)", info.Name, info.Version)
			if err := p.Init(m.bus, nil); err != nil {
				log.Printf("❌ [插件] 启动失败 [%s]: %v", info.ID, err)
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
		}
	}
}

func (m *Manager) startWebhookPlugin(p PluginInfo) {
	for _, evType := range p.Events {
		et := evType
		targetURL := p.URL
		secret := p.Secret
		subID := m.bus.Subscribe(et, func(ev core.Event) {
			go func() {
				payloadData, err := json.Marshal(map[string]interface{}{
					"event":     ev.Type,
					"timestamp": ev.Time.Unix(),
					"payload":   ev.Payload,
				})
				if err != nil {
					return
				}
				req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(payloadData))
				if err != nil {
					return
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "Ani-Go-Webhook/"+p.ID)
				if secret != "" {
					req.Header.Set("X-AniGo-Secret", secret)
				}
				client := httpx.New(10 * time.Second)
				resp, err := client.Do(req)
				if err == nil {
					resp.Body.Close()
				} else {
					log.Printf("⚠️ [插件 %s] Webhook 发送失败: %v", p.Name, err)
				}
			}()
		})
		m.customSubs[p.ID] = append(m.customSubs[p.ID], customSubRef{evType: et, subID: subID})
	}
	log.Printf("🔌 [插件] 已加载自定义 Webhook: %s -> %s (事件: %v)", p.Name, p.URL, p.Events)
}

func (m *Manager) GetPluginList() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]PluginInfo, 0, len(m.builtInPlugins)+len(m.customPlugins))
	for _, p := range m.builtInPlugins {
		info := p.GetInfo()
		enabled, exists := m.activePlugins[info.ID]
		if !exists {
			enabled = true
		}
		info.Enabled = enabled
		list = append(list, info)
	}
	for _, cp := range m.customPlugins {
		enabled, exists := m.activePlugins[cp.ID]
		if !exists {
			enabled = cp.Enabled
		}
		cp.Enabled = enabled
		list = append(list, cp)
	}
	return list
}

func (m *Manager) GetPlugins() interface{} {
	return m.GetPluginList()
}

func (m *Manager) Reload() {
	m.Load()
}

func (m *Manager) GetBus() core.EventBus {
	return m.bus
}

func (m *Manager) TogglePlugin(id string, enabled bool) error {
	m.mu.Lock()
	m.activePlugins[id] = enabled
	data, _ := json.Marshal(m.activePlugins)
	database.DB.Save(&database.Setting{Key: "plugin_status", Value: string(data)})
	m.mu.Unlock()

	m.Load()
	return nil
}

func (m *Manager) AddOrUpdatePlugin(p PluginInfo) error {
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
	if p.Icon == "" {
		p.Icon = "Sparkles"
	}

	m.mu.Lock()
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
	data, _ := json.Marshal(m.customPlugins)
	database.DB.Save(&database.Setting{Key: "custom_plugins", Value: string(data)})
	m.mu.Unlock()

	m.Load()
	return nil
}

func (m *Manager) DeletePlugin(id string) error {
	m.mu.Lock()
	newList := make([]PluginInfo, 0, len(m.customPlugins))
	for _, cp := range m.customPlugins {
		if cp.ID != id {
			newList = append(newList, cp)
		}
	}
	m.customPlugins = newList
	data, _ := json.Marshal(m.customPlugins)
	database.DB.Save(&database.Setting{Key: "custom_plugins", Value: string(data)})
	m.mu.Unlock()

	m.Load()
	return nil
}

