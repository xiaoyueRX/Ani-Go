// Package organizer 实现文件整理功能
// 按照模板变量系统对下载完成的文件进行重命名和目录创建
// v0.5.0 新增: Waterfall 拦截式钩子链
package organizer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// ============================================================
// TVOrganizer 实现 core.Organizer 接口
// ============================================================

type TVOrganizer struct {
	tvTemplate    string
	movieTemplate string
	otherTemplate string
	tvBasePath    string
	movieBasePath string
	ovaBasePath   string
	useHardLink   bool
	eventBus      core.EventBus              // 事件总线，用于触发整理完成事件（松耦合）
	hookManager   *core.WaterfallHookManager // Waterfall 钩子管理器
}

const (
	fallbackTVTemplate      = "{title_cn}{year}/Season {season}/{title_en} [tmdbid={tmdb_id}] S{season:02}E{ep:02}{ext}"
	fallbackMovieTemplate   = "{title_cn} ({year})/{title_en}{ext}"
	fallbackSpecialTemplate = "{title_cn}{year}/Specials/{title_en} S00E{ep:02}{ext}"
)

var _ core.Organizer = (*TVOrganizer)(nil)

// New 创建文件整理器实例
// busOrProvider 允许传入 core.EventBus，或实现了 GetBus() core.EventBus 的管理器，或 nil
func New(tvTemplate, movieTemplate, otherTemplate, tvBasePath, movieBasePath string, useHardLink bool, busOrProvider any, opts ...string) *TVOrganizer {
	ovaPath := "./TV/OVA"
	if len(opts) > 0 && opts[0] != "" {
		ovaPath = opts[0]
	}

	var bus core.EventBus
	if busOrProvider != nil {
		if b, ok := busOrProvider.(core.EventBus); ok {
			bus = b
		} else if bp, ok := busOrProvider.(interface{ GetBus() core.EventBus }); ok {
			bus = bp.GetBus()
		}
	}

	return &TVOrganizer{
		tvTemplate:    tvTemplate,
		movieTemplate: movieTemplate,
		otherTemplate: otherTemplate,
		tvBasePath:    tvBasePath,
		movieBasePath: movieBasePath,
		ovaBasePath:   ovaPath,
		useHardLink:   useHardLink,
		eventBus:      bus,
		hookManager:   core.NewWaterfallHookManager(),
	}
}

// GetHookManager 获取钩子管理器（供插件注册钩子）
func (o *TVOrganizer) GetHookManager() *core.WaterfallHookManager {
	return o.hookManager
}

func (o *TVOrganizer) Name() string { return "TVOrganizer" }

// Organize 整理单个文件：根据模板生成新路径，创建目录，移动/链接文件
func (o *TVOrganizer) Organize(ctx context.Context, filePath string, anime core.Anime, episode core.Episode) (string, error) {
	return o.OrganizeAndCommit(ctx, filePath, anime, episode, func(path string, move *MoveSource) error {
		if move != nil {
			return move.Cleanup(ctx, path)
		}
		return nil
	})
}

// OrganizeAndCommit 先完整发布库文件，再让调用方提交记录。提交失败时保留下载源供重试。
func (o *TVOrganizer) OrganizeAndCommit(ctx context.Context, filePath string, anime core.Anime, episode core.Episode, commit func(string, *MoveSource) error) (string, error) {
	plan, err := o.Preview(ctx, filePath, anime, episode)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	src, err := os.Lstat(filePath)
	if err != nil {
		return "", err
	}
	if !src.Mode().IsRegular() || src.Size() == 0 {
		return "", fmt.Errorf("源文件不是非空普通文件: %s", filePath)
	}
	dst := plan.FinalPath
	if err := makeMediaDir(filepath.Dir(dst)); err != nil {
		return "", err
	}
	existing, err := os.Lstat(dst)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if existing == nil || !os.SameFile(src, existing) {
		if err := installMedia(ctx, filePath, dst); err != nil {
			return "", err
		}
	}
	if err := syncMediaDir(filepath.Dir(dst)); err != nil {
		return "", err
	}
	var move *MoveSource
	if plan.Action == "move" && !sameDirectoryEntry(filePath, dst) {
		source, err := filepath.Abs(filePath)
		if err != nil {
			return "", err
		}
		move = &MoveSource{Path: source, ModTime: src.ModTime().UnixNano()}
	}
	if err := commit(dst, move); err != nil {
		return "", err
	}
	o.publishOrganizedEvent(filePath, dst, anime, episode)
	return dst, nil
}

func (o *TVOrganizer) publishOrganizedEvent(filePath, fullPath string, anime core.Anime, episode core.Episode) {
	if o.eventBus != nil {
		o.eventBus.Publish(core.Event{
			Type: core.EventFileOrganized,
			Payload: map[string]any{
				"anime":      anime,
				"episode":    episode,
				"old_path":   filePath,
				"new_path":   fullPath,
				"anime_type": anime.Type,
			},
			Time: time.Now(),
		})
	}
}

// normalizeAnimeType 规范化番剧分类类型（大小写无关，支持常见别名）
func normalizeAnimeType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "movie", "film", "劇場版", "剧场版":
		return "movie"
	case "tv", "series", "animation", "scripted", "miniseries":
		return "tv"
	case "ova", "oad", "special", "sp", "extra":
		return "ova"
	default:
		return "other"
	}
}

// selectTemplate 根据类型选择模板
func (o *TVOrganizer) selectTemplate(anime core.Anime) string {
	normType := normalizeAnimeType(anime.Type)
	switch normType {
	case "movie":
		if database.DB != nil {
			var setting database.Setting
			if err := database.DB.Where("key = ?", "MOVIE_TEMPLATE").First(&setting).Error; err == nil && setting.Value != "" {
				return setting.Value
			}
		}
		if o.movieTemplate != "" {
			return o.movieTemplate
		}
		return fallbackMovieTemplate
	case "tv":
		if database.DB != nil {
			var setting database.Setting
			if err := database.DB.Where("key = ?", "TV_TEMPLATE").First(&setting).Error; err == nil && setting.Value != "" {
				return setting.Value
			}
		}
		if o.tvTemplate != "" {
			return o.tvTemplate
		}
		return fallbackTVTemplate
	default:
		// OVA, Special, 或未指定类型 (other)
		if database.DB != nil {
			var setting database.Setting
			if err := database.DB.Where("key = ?", "OTHER_TEMPLATE").First(&setting).Error; err == nil && setting.Value != "" {
				return setting.Value
			}
		}
		if o.otherTemplate != "" {
			return o.otherTemplate
		}
		if o.tvTemplate != "" {
			return o.tvTemplate
		}
		return fallbackSpecialTemplate
	}
}

func (o *TVOrganizer) getBasePath(animeType string) string {
	normType := normalizeAnimeType(animeType)
	var basePath string
	switch normType {
	case "movie":
		basePath = o.movieBasePath
		if database.DB != nil {
			var movieBaseSetting database.Setting
			if err := database.DB.Where("key = ?", "MOVIE_BASE_PATH").First(&movieBaseSetting).Error; err == nil && movieBaseSetting.Value != "" {
				basePath = movieBaseSetting.Value
			}
		}
		if basePath == "" {
			basePath = "./TV/Media/剧场版"
		}
	case "ova":
		basePath = o.ovaBasePath
		if database.DB != nil {
			var ovaBaseSetting database.Setting
			if err := database.DB.Where("key = ?", "OVA_BASE_PATH").First(&ovaBaseSetting).Error; err == nil && ovaBaseSetting.Value != "" {
				basePath = ovaBaseSetting.Value
			}
		}
		if basePath == "" {
			basePath = "./TV/Media/OVA"
		}
	default:
		basePath = o.tvBasePath
		if database.DB != nil {
			var baseSetting database.Setting
			if err := database.DB.Where("key = ?", "TV_BASE_PATH").First(&baseSetting).Error; err == nil && baseSetting.Value != "" {
				basePath = baseSetting.Value
			}
		}
		if basePath == "" {
			basePath = "./TV/Media/番剧"
		}
	}
	return basePath
}

func (o *TVOrganizer) isHardLinkEnabled() bool {
	useHardLink := o.useHardLink
	if database.DB != nil {
		var hlSetting database.Setting
		if err := database.DB.Where("key = ?", "USE_HARDLINK").First(&hlSetting).Error; err == nil {
			useHardLink = (hlSetting.Value == "true" || hlSetting.Value == "1")
		}
	}
	return useHardLink
}

// namingInput 为预览和实际整理构造相同的初始路径，并向钩子保留原始语言元数据。
func (o *TVOrganizer) namingInput(filePath string, anime core.Anime, episode core.Episode) core.NamingHookInput {
	template := o.selectTemplate(anime)
	values := o.buildVarValues(anime, episode)
	values.Ext = filepath.Ext(filePath)
	renderValues := values
	renderLayout := template
	switch template {
	case config.DefaultTVTemplate, config.DefaultMovieTemplate, fallbackTVTemplate, fallbackMovieTemplate, fallbackSpecialTemplate:
		// 内置目录名优先中文，文件名优先英文。自定义模板中的语言占位符保持原义。
		renderValues.TitleCN = firstTitle(anime.TitleCN, anime.TitleJP, anime.TitleEN)
		renderValues.TitleEN = firstTitle(anime.TitleEN, anime.TitleCN, anime.TitleJP)
		if template == fallbackTVTemplate && resolveVar("tmdb_id", values) == "" {
			renderLayout = strings.Replace(template, " [tmdbid={tmdb_id}]", "", 1)
		}
	}
	return core.NamingHookInput{
		Anime: anime, Episode: episode, Template: template, VarValues: values,
		RenderedPath: renderTemplate(renderLayout, renderValues),
	}
}

func firstTitle(candidates ...string) string {
	for _, candidate := range candidates {
		if title := strings.TrimSpace(candidate); title != "" {
			return title
		}
	}
	return ""
}

// buildVarValues 从 anime 和 episode 构建模板变量值
func (o *TVOrganizer) buildVarValues(anime core.Anime, episode core.Episode) core.VarValues {
	series := anime.SeriesID
	if series == "" {
		series = anime.TitleCN
	}
	if series == "" {
		series = anime.TitleJP
	}
	if series == "" {
		series = anime.TitleEN
	}
	return core.VarValues{
		TitleCN:  anime.TitleCN,
		TitleEN:  anime.TitleEN,
		Year:     anime.Year,
		Season:   episode.Season,
		Ep:       episode.Number,
		Ext:      "",
		AnimeID:  anime.ID,
		Provider: anime.Provider,
		TMDBID:   anime.TMDBID,
		IMDBID:   anime.IMDBID,
		SeriesID: anime.SeriesID,
		Series:   series,
	}
}

// ============================================================
// 模板渲染引擎
// ============================================================

// 模板变量正则：匹配 {var_name} 和 {var_name:format}
var reTemplateVar = regexp.MustCompile(`\{(\w+)(?::(\w+))?\}`)
var yearSegmentPattern = regexp.MustCompile(`(?m)\s*\((?:0|)\)`)

// removeYearSegment removes the optional parentheses around {year} when
// the value is absent (0 or empty), while preserving unrelated literal parentheses.
func removeYearSegment(path string) string {
	trimmed := yearSegmentPattern.ReplaceAllString(path, "")
	return strings.TrimSpace(trimmed)
}

// renderTemplate 将模板字符串渲染为实际路径
func renderTemplate(template string, v core.VarValues) string {
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

	if v.Year <= 0 {
		result = removeYearSegment(result)
	}

	// 清理非法文件名字符
	result = sanitizePath(result)

	return result
}

// checkPathEscape 防御性防止任何非法路径穿越逃逸 basePath
func checkPathEscape(basePath, fullPath string) error {
	absBase, err1 := filepath.Abs(basePath)
	absTarget, err2 := filepath.Abs(fullPath)
	if err1 != nil || err2 != nil {
		return fmt.Errorf("解析路径失败: %s", fullPath)
	}
	cleanBase := filepath.Clean(absBase)
	cleanTarget := filepath.Clean(absTarget)
	rel, relErr := filepath.Rel(cleanBase, cleanTarget)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == "." {
		return fmt.Errorf("非法目标路径越界: %s", fullPath)
	}
	return nil
}

// cleanVarValue 清理变量值中的路径穿越符与控制字符
func cleanVarValue(val string) string {
	val = strings.ReplaceAll(val, "\x00", "")
	val = strings.ReplaceAll(val, "\r", "")
	val = strings.ReplaceAll(val, "\n", "")
	for strings.Contains(val, "..") {
		val = strings.ReplaceAll(val, "..", "")
	}
	return val
}

// resolveVar 根据变量名获取对应的值
func resolveVar(name string, v core.VarValues) string {
	switch name {
	case "title_cn":
		return cleanVarValue(v.TitleCN)
	case "title_en":
		return cleanVarValue(v.TitleEN)
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
		return cleanVarValue(v.Ext)
	case "tmdb_id":
		id := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(v.TMDBID), "tv/"), "movie/")
		return cleanVarValue(id)
	case "imdb_id":
		return cleanVarValue(v.IMDBID)
	case "series":
		return cleanVarValue(v.Series)
	case "series_id":
		return cleanVarValue(v.SeriesID)
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
