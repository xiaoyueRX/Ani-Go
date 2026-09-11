package organizer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
	template := o.selectTemplate(anime)
	values := o.buildVarValues(anime, episode)

	// 初始渲染
	initialPath := renderTemplate(template, values)

	// 执行命名钩子链（Waterfall 模式）
	var finalPath string
	if o.hookManager != nil {
		hookFinal, cancelled, reason := o.hookManager.ExecuteNamingHooks(ctx, core.NamingHookInput{
			Anime:        anime,
			Episode:      episode,
			Template:     template,
			VarValues:    values,
			RenderedPath: initialPath,
		})
		if cancelled {
			return PreviewResult{
				OriginalPath: filePath,
				RenderedPath: initialPath,
				Action:       "cancelled",
				Template:     template,
				Error:        reason,
			}, fmt.Errorf("整理被钩子取消: %s", reason)
		}
		finalPath = hookFinal
	} else {
		finalPath = initialPath
	}

	basePath := o.getBasePath(anime.Type)
	fullPath := filepath.Join(basePath, finalPath)

	// 补充扩展名
	if filepath.Ext(fullPath) == "" && filePath != "" {
		fullPath += filepath.Ext(filePath)
	}

	// 边界检查：防御性防止任何非法路径穿越逃逸 basePath（在最终完整路径上校验）
	if err := checkPathEscape(basePath, fullPath); err != nil {
		return PreviewResult{
			OriginalPath: filePath,
			RenderedPath: initialPath,
			FinalPath:    fullPath,
			Template:     template,
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
		RenderedPath: initialPath,
		FinalPath:    fullPath,
		Action:       action,
		Exists:       exists,
		Template:     template,
	}, nil
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
