// Package organizer 实现文件整理功能
// 按照模板变量系统对下载完成的文件进行重命名和目录创建
package organizer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// ============================================================
// TVOrganizer 实现 core.Organizer 接口
// ============================================================

type TVOrganizer struct {
	tvTemplate    string
	movieTemplate string
	tvBasePath    string
	movieBasePath string
	useHardLink   bool
}

// New 创建文件整理器实例
func New(tvTemplate, movieTemplate, tvBasePath, movieBasePath string, useHardLink bool) *TVOrganizer {
	return &TVOrganizer{
		tvTemplate:    tvTemplate,
		movieTemplate: movieTemplate,
		tvBasePath:    tvBasePath,
		movieBasePath: movieBasePath,
		useHardLink:   useHardLink,
	}
}

func (o *TVOrganizer) Name() string { return "TVOrganizer" }

// Organize 整理单个文件：根据模板生成新路径，创建目录，移动/链接文件
func (o *TVOrganizer) Organize(ctx context.Context, filePath string, anime core.Anime, episode core.Episode) (string, error) {
	template := o.selectTemplate(anime)
	values := o.buildVarValues(anime, episode)

	newPath := renderTemplate(template, values)

	// 确保目标路径是绝对路径
	basePath := o.tvBasePath
	if anime.Type == "Movie" {
		basePath = o.movieBasePath
	}
	fullPath := filepath.Join(basePath, newPath)

	// 补充扩展名
	if filepath.Ext(fullPath) == "" {
		fullPath += filepath.Ext(filePath)
	}

	// 创建目标目录
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	// 移动或硬链接文件
	if o.useHardLink {
		if err := os.Link(filePath, fullPath); err != nil {
			return "", fmt.Errorf("创建硬链接失败: %w", err)
		}
	} else {
		if err := os.Rename(filePath, fullPath); err != nil {
			return "", fmt.Errorf("移动文件失败: %w", err)
		}
	}

	return fullPath, nil
}

// selectTemplate 根据类型选择模板
func (o *TVOrganizer) selectTemplate(anime core.Anime) string {
	switch anime.Type {
	case "Movie":
		if o.movieTemplate != "" {
			return o.movieTemplate
		}
		return "{title_cn} ({year})/{title_en}{ext}"
	default:
		if o.tvTemplate != "" {
			return o.tvTemplate
		}
		return "{title_cn} ({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}"
	}
}

// VarValues 保存模板变量名到值的映射
type VarValues struct {
	TitleCN  string
	TitleEN  string
	Year     int
	Season   int
	Ep       float32
	Ext      string
	AnimeID  string
	Provider string
}

// buildVarValues 从 anime 和 episode 构建模板变量值
func (o *TVOrganizer) buildVarValues(anime core.Anime, episode core.Episode) VarValues {
	return VarValues{
		TitleCN:  anime.TitleCN,
		TitleEN:  anime.TitleEN,
		Year:     anime.Year,
		Season:   episode.Season,
		Ep:       episode.Number,
		Ext:      "",
		AnimeID:  anime.ID,
		Provider: anime.Provider,
	}
}

// ============================================================
// 模板渲染引擎
// ============================================================

// 模板变量正则：匹配 {var_name} 和 {var_name:format}
var reTemplateVar = regexp.MustCompile(`\{(\w+)(?::(\w+))?\}`)

// renderTemplate 将模板字符串渲染为实际路径
func renderTemplate(template string, v VarValues) string {
	result := reTemplateVar.ReplaceAllStringFunc(template, func(match string) string {
		// 提取变量名和格式
		parts := reTemplateVar.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		varName := parts[1]
		format := ""
		if len(parts) > 2 {
			format = parts[2]
		}

		// 根据变量名获取值
		val := resolveVar(varName, v)

		// 应用格式
		return applyFormat(val, format)
	})

	// 清理非法文件名字符
	result = sanitizePath(result)

	return result
}

// resolveVar 根据变量名获取对应的值
func resolveVar(name string, v VarValues) string {
	switch name {
	case "title_cn":
		return v.TitleCN
	case "title_en":
		return v.TitleEN
	case "year":
		if v.Year > 0 {
			return fmt.Sprintf("%d", v.Year)
		}
		return ""
	case "season":
		return fmt.Sprintf("%d", v.Season)
	case "ep":
		return fmt.Sprintf("%02g", v.Ep)
	case "ext":
		return v.Ext
	default:
		return "{" + name + "}"
	}
}

// applyFormat 对变量值应用格式化（如 :02 表示补零）
func applyFormat(val, format string) string {
	switch format {
	case "02":
		// 两位补零
		if len(val) == 1 {
			return "0" + val
		}
		return val
	default:
		return val
	}
}

// sanitizePath 移除路径中的非法字符
func sanitizePath(path string) string {
	// Windows 和 Linux 的非法字符
	illegal := []string{`<`, `>`, `:`, `"`, `|`, `?`, `*`, "\x00"}
	result := path
	for _, ch := range illegal {
		result = strings.ReplaceAll(result, ch, "")
	}
	// 去除首尾空格和点
	result = strings.TrimSpace(result)
	result = strings.Trim(result, ".")
	return result
}
