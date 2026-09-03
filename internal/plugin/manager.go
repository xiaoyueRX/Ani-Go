package plugin

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

type Manager struct {
	bus            core.EventBus
	mu             sync.RWMutex
	builtInPlugins []BuiltInPlugin
	activePlugins  map[string]bool // id -> enabled
}

func NewManager(bus core.EventBus) *Manager {
	m := &Manager{
		bus:           bus,
		activePlugins: make(map[string]bool),
	}
	// 注册内建插件
	m.builtInPlugins = append(m.builtInPlugins, &MetadataMappingPlugin{})
	return m
}

func (m *Manager) Load() {
	m.mu.Lock()
	defer m.mu.Unlock()

	var setting database.Setting
	if err := database.DB.Where("key = ?", "plugin_status").First(&setting).Error; err == nil {
		json.Unmarshal([]byte(setting.Value), &m.activePlugins)
	}

	for _, p := range m.builtInPlugins {
		info := p.GetInfo()
		// 默认开启元数据映射插件，或者根据数据库配置开启
		enabled, exists := m.activePlugins[info.ID]
		if !exists {
			enabled = true // 默认内建插件开启
		}

		if enabled {
			log.Printf("🔌 [插件] 正在启动: %s (%s)", info.Name, info.Version)
			if err := p.Init(m.bus, nil); err != nil {
				log.Printf("❌ [插件] 启动失败 [%s]: %v", info.ID, err)
			}
		}
	}
}

func (m *Manager) GetPluginList() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]PluginInfo, 0, len(m.builtInPlugins))
	for _, p := range m.builtInPlugins {
		info := p.GetInfo()
		enabled, exists := m.activePlugins[info.ID]
		if !exists {
			enabled = true
		}
		info.Enabled = enabled
		list = append(list, info)
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
	return nil
}
