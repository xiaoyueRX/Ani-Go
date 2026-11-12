package plugin

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

var (
	// DefaultGeekNamingPlugin 媒体库规范重命名插件共享单例
	DefaultGeekNamingPlugin = &GeekNamingPlugin{enabled: true}
)

// GeekNamingPlugin 媒体库规范重命名插件
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

// InitGeekNaming 注册规范命名钩子（重复调用时先清理同名旧钩子以保证幂等）
func InitGeekNaming(mgr *core.WaterfallHookManager) {
	if mgr == nil {
		return
	}
	mgr.UnregisterHooksByName("StandardNaming")
	mgr.RegisterNamingHook(core.PriorityHigh, "StandardNaming", func(ctx context.Context, input interface{}) (interface{}, error) {
		in, ok := input.(core.NamingHookInput)
		if !ok {
			return nil, fmt.Errorf("规范命名钩子收到无效输入类型: %T", input)
		}
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

var mediaServerReplacer = strings.NewReplacer(
	":", " - ",
	"?", "？",
	"*", " ",
	"\"", "'",
	"<", "（",
	">", "）",
	"|", " - ",
)

// sanitizePathForMediaServer 清洗整理路径中的跨平台非法字符与多余空格，并保留绝对路径根前缀
func sanitizePathForMediaServer(p string) string {
	if strings.TrimSpace(p) == "" {
		return p
	}
	clean := filepath.Clean(p)
	if clean == "." || clean == string(filepath.Separator) {
		return clean
	}

	vol := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, vol)
	hasRootSep := strings.HasPrefix(rest, string(filepath.Separator))
	trimmed := strings.TrimPrefix(rest, string(filepath.Separator))
	if trimmed == "" {
		return clean
	}

	rawParts := strings.Split(trimmed, string(filepath.Separator))
	cleanedParts := make([]string, 0, len(rawParts))
	for _, seg := range rawParts {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			cleanedParts = append(cleanedParts, seg)
			continue
		}
		s := mediaServerReplacer.Replace(seg)
		fields := strings.Fields(s)
		if len(fields) > 0 {
			s = strings.Join(fields, " ")
		} else {
			s = ""
		}
		// 移除文件名或目录末尾的空格与句点（Windows 规范）
		s = strings.TrimRight(s, " .")
		if s == "" {
			s = "_"
		}
		cleanedParts = append(cleanedParts, s)
	}

	joined := filepath.Join(cleanedParts...)
	if hasRootSep {
		return vol + string(filepath.Separator) + joined
	}
	if vol != "" {
		return vol + joined
	}
	return joined
}
