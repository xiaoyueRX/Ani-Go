package plugin

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

var (
	DefaultGeekNamingPlugin = &GeekNamingPlugin{enabled: true}
)

type GeekNamingPlugin struct {
	mu      sync.RWMutex
	enabled bool
}

func (p *GeekNamingPlugin) GetInfo() PluginInfo {
	return PluginInfo{
		ID:          "standard-naming",
		Name:        "媒体库规范重命名",
		Description: "在番剧入库与文件整理时，遵循 Plex/Emby/Jellyfin 刮削规范，自动格式化文件名与季集目录结构。",
		Version:     "1.0.0",
		Author:      "xiaoyue",
		AuthorURL:   "https://github.com/xiaoyueRX",
		Icon:        "FolderCheck",
		IsBuiltIn:   true,
		Type:        "builtin",
		Events:      []string{core.EventFileOrganized},
	}
}

func (p *GeekNamingPlugin) Init(bus core.EventBus, ctx core.Context) error {
	p.mu.Lock()
	p.enabled = true
	p.mu.Unlock()
	log.Println("🔌 [插件] 媒体库规范重命名插件已启用")
	return nil
}

func (p *GeekNamingPlugin) Stop(bus core.EventBus) error {
	p.mu.Lock()
	p.enabled = false
	p.mu.Unlock()
	log.Println("🔌 [插件] 媒体库规范重命名插件已停用，钩子将跳过规范化处理")
	return nil
}

// InitGeekNaming 注册规范命名插件
func InitGeekNaming(mgr *core.WaterfallHookManager) {
	mgr.RegisterNamingHook(core.PriorityHigh, "StandardNaming", func(ctx context.Context, input interface{}) (interface{}, error) {
		in := input.(core.NamingHookInput)
		DefaultGeekNamingPlugin.mu.RLock()
		active := DefaultGeekNamingPlugin.enabled
		DefaultGeekNamingPlugin.mu.RUnlock()
		if !active {
			return core.NamingHookOutput{
				RenderedPath: in.RenderedPath,
			}, nil
		}
		sanitized := sanitizePathForMediaServer(in.RenderedPath)
		if sanitized != in.RenderedPath {
			log.Printf("📁 [规范命名] 优化文件路径: %s -> %s", in.RenderedPath, sanitized)
		}
		return core.NamingHookOutput{
			RenderedPath: sanitized,
		}, nil
	})
}

// sanitizePathForMediaServer 清洗整理路径中的跨平台非法字符与多余空格，遵循 Plex/Emby/Jellyfin 刮削规范
func sanitizePathForMediaServer(p string) string {
	clean := filepath.Clean(p)
	parts := strings.Split(clean, string(filepath.Separator))
	for i, seg := range parts {
		if i == 0 && (strings.Contains(seg, ":") && len(seg) == 2) {
			// Windows 盘符如 C: 保留
			continue
		}
		// 替换文件系统非法字符
		r := strings.NewReplacer(
			":", " - ",
			"?", "？",
			"*", " ",
			"\"", "'",
			"<", "（",
			">", "）",
			"|", " - ",
		)
		s := r.Replace(seg)
		// 合并多余空格
		fields := strings.Fields(s)
		if len(fields) > 0 {
			s = strings.Join(fields, " ")
		}
		// 移除文件名或目录末尾的空格与句点（Windows 规范）
		s = strings.TrimRight(s, " .")
		if s != "" {
			parts[i] = s
		}
	}
	return filepath.Join(parts...)
}
