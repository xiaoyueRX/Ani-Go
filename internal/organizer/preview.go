package organizer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// PreviewItem 批量预览请求项
type PreviewItem struct {
	FilePath string       `json:"file_path"`
	Anime    core.Anime   `json:"anime"`
	Episode  core.Episode `json:"episode"`
}

// PreviewResult 整理 Dry-Run 预览结果
type PreviewResult struct {
	OriginalPath string `json:"original_path"`
	RenderedPath string `json:"rendered_path"`
	FinalPath    string `json:"final_path"`
	Action       string `json:"action"` // "hardlink", "move", "cancelled"
	Exists       bool   `json:"exists"` // 目标路径是否已存在同名文件
	Template     string `json:"template"`
	Error        string `json:"error,omitempty"`
}

// Preview 执行单个文件的 Dry-Run 预览，不执行任何文件系统写操作
func (o *TVOrganizer) Preview(ctx context.Context, filePath string, anime core.Anime, episode core.Episode) (PreviewResult, error) {
	input, roots := o.namingInput(filePath, anime, episode)
	finalPath := input.RenderedPath
	if o.hookManager != nil {
		hookFinal, cancelled, reason := o.hookManager.ExecuteNamingHooks(ctx, input)
		if cancelled {
			return PreviewResult{
				OriginalPath: filePath,
				RenderedPath: input.RenderedPath,
				Action:       "cancelled",
				Template:     input.Template,
				Error:        reason,
			}, fmt.Errorf("整理被钩子取消: %s", reason)
		}
		finalPath = hookFinal
	}

	basePath := o.getBasePath(anime.Type)
	if roots.current != "" {
		finalPath = roots.existingPath(basePath, finalPath)
	}
	fullPath := filepath.Join(basePath, finalPath)

	// 补充扩展名
	if filepath.Ext(fullPath) == "" && filePath != "" {
		fullPath += filepath.Ext(filePath)
	}

	// 边界检查：防御性防止任何非法路径穿越逃逸 basePath（在最终完整路径上校验）
	if err := checkPathEscape(basePath, fullPath); err != nil {
		return PreviewResult{
			OriginalPath: filePath,
			RenderedPath: input.RenderedPath,
			FinalPath:    fullPath,
			Template:     input.Template,
			Error:        err.Error(),
		}, err
	}

	action := "move"
	if o.isHardLinkEnabled() {
		action = "hardlink"
	}

	// 检查目标文件是否已存在
	_, statErr := os.Stat(fullPath)
	exists := (statErr == nil)

	return PreviewResult{
		OriginalPath: filePath,
		RenderedPath: input.RenderedPath,
		FinalPath:    fullPath,
		Action:       action,
		Exists:       exists,
		Template:     input.Template,
	}, nil
}

// existingPath 按整段剧根保留已入库的旧布局，下一季尚无目录也可续写。
func (roots tvSeriesRoots) existingPath(base, final string) string {
	current := filepath.ToSlash(filepath.Clean(roots.current))
	legacy := filepath.ToSlash(filepath.Clean(roots.legacy))
	path := filepath.ToSlash(filepath.Clean(final))
	rest, ok := strings.CutPrefix(path, current+"/")
	if !ok {
		// 标准钩子逐层规范化路径；其他钩子另选的剧根优先。
		current = filepath.ToSlash(core.SanitizePathForMediaServer(roots.current))
		legacy = filepath.ToSlash(core.SanitizePathForMediaServer(roots.legacy))
		rest, ok = strings.CutPrefix(path, current+"/")
	}
	if !ok || !fs.ValidPath(current) || !fs.ValidPath(legacy) ||
		!filepath.IsLocal(current) || !filepath.IsLocal(legacy) || filepath.Dir(current) != filepath.Dir(legacy) {
		return final
	}
	// 名字含分隔符时，避免通过父目录的符号链接去探测或采用旧剧根。
	dir := base
	parts := strings.Split(current, "/")
	for _, part := range parts[:len(parts)-1] {
		dir = filepath.Join(dir, part)
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() {
			return final
		}
	}
	if _, err := os.Lstat(filepath.Join(base, current)); !os.IsNotExist(err) {
		return final
	}
	if old, err := os.Lstat(filepath.Join(base, legacy)); err == nil && old.IsDir() {
		return filepath.Join(legacy, filepath.FromSlash(rest))
	}
	return final
}

// PreviewBatch 批量预览多个文件的整理路径
func (o *TVOrganizer) PreviewBatch(ctx context.Context, items []PreviewItem) ([]PreviewResult, error) {
	results := make([]PreviewResult, 0, len(items))
	for _, item := range items {
		res, err := o.Preview(ctx, item.FilePath, item.Anime, item.Episode)
		if err != nil && res.Error == "" {
			res.Error = err.Error()
		}
		results = append(results, res)
	}
	return results, nil
}
