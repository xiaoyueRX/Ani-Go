package plugin

import "github.com/xiaoyueRX/Ani-Go/internal/core"

// PluginInfo 插件元数据
type PluginInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	AuthorURL   string   `json:"author_url"`
	Icon        string   `json:"icon"` // Lucide 图标名称或 URL
	Enabled     bool     `json:"enabled"`
	IsBuiltIn   bool     `json:"is_builtin"`
	Type        string   `json:"type"` // "builtin", "webhook", "script"
	URL         string   `json:"url,omitempty"`
	Secret      string   `json:"secret,omitempty"`
	Events      []string `json:"events"`
}

// BuiltInPlugin 内建插件接口
type BuiltInPlugin interface {
	GetInfo() PluginInfo
	Init(bus core.EventBus, ctx core.Context) error
}
