package plugin

import (
	"errors"
	"fmt"
	"strings"
)

// 来源由服务端注册清单决定，作者署名由插件保留。
const (
	OriginOfficial  = "official"
	OriginCommunity = "community"
)

var (
	ErrReservedPluginID      = errors.New("插件 ID 已由内置插件保留")
	ErrInvalidPluginManifest = errors.New("插件清单无效")
)

var officialPluginIDs = map[string]bool{
	"metadata-mapping":   true,
	"standard-naming":    true,
	"yuc_schedule":       true,
	"data-migrate":       true,
	"extended-notifiers": true,
	"nfo_scraper":        true,
	Cloud115ID:           true,
}

// IsOfficialPluginID 识别正式内置插件的规范 ID，供旧版备份恢复开关使用。
func IsOfficialPluginID(id string) bool {
	return officialPluginIDs[id]
}

func (m *Manager) reservedPluginID(id string) bool {
	id = strings.TrimSpace(id)
	if officialPluginIDs[strings.ToLower(id)] {
		return true
	}
	for _, p := range m.builtInPlugins {
		if strings.EqualFold(id, p.GetInfo().ID) {
			return true
		}
	}
	return false
}

// 旧版本允许自定义插件和内置插件同名。保留配置和署名，分配独立 ID，
// 并解除来源不明确的共享开关，等待用户分别启用。
func (m *Manager) normalizeCustomPlugins(plugins []PluginInfo, states map[string]bool) bool {
	taken := make(map[string]bool, len(plugins)+len(states))
	for _, p := range plugins {
		taken[strings.ToLower(p.ID)] = true
	}
	for id := range states {
		taken[strings.ToLower(id)] = true
	}

	rekeyed := false
	for i := range plugins {
		p := &plugins[i]
		p.Origin = OriginCommunity
		p.IsBuiltIn = false
		if strings.EqualFold(p.Type, "builtin") {
			p.Type = "custom"
		}
		if !m.reservedPluginID(p.ID) {
			continue
		}

		base := "custom-" + strings.ToLower(strings.TrimSpace(p.ID))
		id := base
		for suffix := 2; taken[id] || m.reservedPluginID(id); suffix++ {
			id = fmt.Sprintf("%s-%d", base, suffix)
		}
		if _, shared := states[p.ID]; shared {
			states[p.ID] = false
		}
		p.ID, p.Enabled = id, false
		taken[id] = true
		rekeyed = true
	}
	return rekeyed
}
