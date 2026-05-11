// Package plugin 提供插件管理器
// 支持 Webhook 插件和 Shell 脚本插件，通过 EventBus 事件触发
package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// ============================================================
// 插件类型定义
// ============================================================

type PluginType string

const (
	TypeWebhook PluginType = "webhook"
	TypeScript  PluginType = "script"
)

type PluginConfig struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`    // "webhook" | "script"
	URL     string   `json:"url,omitempty"`
	Command string   `json:"command,omitempty"`
	Events  []string `json:"events"`
	Enabled bool     `json:"enabled"`
}

// Manager 插件管理器
type Manager struct {
	bus      core.EventBus
	mu       sync.RWMutex
	plugins  []PluginConfig
	client   *http.Client
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewManager 创建插件管理器
func NewManager(bus core.EventBus) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		bus:    bus,
		client: &http.Client{Timeout: 15 * time.Second},
		ctx:    ctx,
		cancel: cancel,
	}
}

// LoadFromSettings 从 settings 表加载插件配置
func (m *Manager) LoadFromSettings() {
	m.mu.Lock()
	defer m.mu.Unlock()

	var settings []database.Setting
	database.DB.Where("key LIKE ?", "plugin\\_%").Find(&settings)
	if len(settings) == 0 {
		m.plugins = nil
		return
	}

	// 将 settings 按 key 索引到槽位
	slotMap := make(map[int]database.Setting, len(settings))
	for _, s := range settings {
		var idx int
		if n, err := fmt.Sscanf(s.Key, "plugin_%d", &idx); err == nil && n == 1 {
			slotMap[idx] = s
		}
	}

	var configs []PluginConfig
	for i := 0; i < 20; i++ {
		setting, ok := slotMap[i]
		if !ok {
			continue
		}
		var cfg PluginConfig
		if err := json.Unmarshal([]byte(setting.Value), &cfg); err != nil {
			log.Printf("⚠️  插件配置解析失败 [%s]: %v", setting.Key, err)
			continue
		}
		if cfg.Name == "" {
			continue
		}
		if cfg.Type == "" {
			cfg.Type = string(TypeWebhook)
		}
		configs = append(configs, cfg)
	}

	m.plugins = configs
	if len(m.plugins) > 0 {
		log.Printf("🔌 已加载 %d 个插件", len(m.plugins))
	}
}

// SubscribeAll 订阅所有插件关注的事件
func (m *Manager) SubscribeAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	eventSet := make(map[string]bool)
	for _, p := range m.plugins {
		if !p.Enabled {
			continue
		}
		for _, ev := range p.Events {
			eventSet[ev] = true
		}
	}

	for ev := range eventSet {
		ev := ev
		m.bus.Subscribe(ev, func(event core.Event) {
			m.handleEvent(event)
		})
	}
}

// Reload 重新加载插件配置
func (m *Manager) Reload() {
	m.LoadFromSettings()
	m.SubscribeAll()
}

func (m *Manager) handleEvent(event core.Event) {
	m.mu.RLock()
	plugins := make([]PluginConfig, len(m.plugins))
	copy(plugins, m.plugins)
	m.mu.RUnlock()

	for _, p := range plugins {
		if !p.Enabled {
			continue
		}
		if !containsEvent(p.Events, event.Type) {
			continue
		}
		go m.executePlugin(p, event)
	}
}

func (m *Manager) executePlugin(p PluginConfig, event core.Event) {
	eventJSON, _ := json.Marshal(event)
	switch PluginType(p.Type) {
	case TypeWebhook:
		m.executeWebhook(p, eventJSON)
	case TypeScript:
		m.executeScript(p, eventJSON)
	default:
		log.Printf("⚠️  未知插件类型: %s", p.Type)
	}
}

func (m *Manager) executeWebhook(p PluginConfig, payload []byte) {
	req, err := http.NewRequestWithContext(m.ctx, http.MethodPost, p.URL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("⚠️  插件 [%s] 请求创建失败: %v", p.Name, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		log.Printf("⚠️  插件 [%s] Webhook 调用失败: %v", p.Name, err)
		return
	}
	resp.Body.Close()
	log.Printf("🔌 插件 [%s] Webhook → %s (状态码 %d)", p.Name, p.URL, resp.StatusCode)
}

func (m *Manager) executeScript(p PluginConfig, payload []byte) {
	cmd := exec.CommandContext(m.ctx, "sh", "-c", p.Command)
	cmd.Stdin = bytes.NewReader(payload)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		log.Printf("⚠️  插件 [%s] 脚本执行失败: %v | stderr: %s", p.Name, err, stderr.String())
		return
	}
	log.Printf("🔌 插件 [%s] 脚本 → %s | 输出: %s", p.Name, p.Command, strings.TrimSpace(string(out)))
}

// GetPlugins 返回当前已加载的插件列表
func (m *Manager) GetPlugins() []PluginConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]PluginConfig, len(m.plugins))
	copy(result, m.plugins)
	return result
}

// Stop 停止管理器
func (m *Manager) Stop() {
	m.cancel()
}

func containsEvent(events []string, target string) bool {
	for _, e := range events {
		if e == target {
			return true
		}
	}
	return false
}
