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
	legacyTVTemplate        = "{title_cn}{year}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}"
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
	return o.organizeAndCommit(ctx, filePath, anime, episode, "", commit)
}

func (o *TVOrganizer) organizeAndCommit(ctx context.Context, filePath string, anime core.Anime, episode core.Episode, versionKey string, commit func(string, *MoveSource) error) (string, error) {
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
	volume, err := (LibraryVolume{Root: plan.basePath, ID: volumeID(anime.Type)}).hold(dst)
	if err != nil {
		return "", err
	}
	defer volume.close()
	if err := makeMediaDir(filepath.Dir(dst)); err != nil {
		return "", err
	}
	guardedPublish := func(stage, target string) error {
		if err := volume.check(target); err != nil {
			return err
		}
		if versionKey != "" {
			return publishNewFile(stage, target)
		}
		return os.Rename(stage, target)
	}
	if versionKey != "" {
		dst, err = publishVersion(ctx, filePath, dst, versionKey, guardedPublish)
	} else {
		var existing os.FileInfo
		existing, err = os.Lstat(dst)
		if os.IsNotExist(err) || (err == nil && !os.SameFile(src, existing)) {
			err = installMedia(ctx, filePath, dst, guardedPublish)
		}
	}
	if err != nil {
		return "", err
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
		root, err := filepath.Abs(plan.basePath)
		if err != nil {
			return "", err
		}
		move = &MoveSource{Path: source, ModTime: src.ModTime().UnixNano(), Volume: LibraryVolume{Root: root}}
		if volume != nil {
			move.Volume = volume.volume
		}
	}
	if err := volume.check(dst); err != nil {
		return "", err
	}
	if err := commit(dst, move); err != nil {
		return "", err
	}
	o.publishOrganizedEvent(filePath, dst, anime, episode, volume)
	return dst, nil
}

func (o *TVOrganizer) publishOrganizedEvent(filePath, fullPath string, anime core.Anime, episode core.Episode, volume *heldVolume) {
	if o.eventBus != nil {
		ev := core.Event{
			Type: core.EventFileOrganized,
			Payload: map[string]any{
				"anime":      anime,
				"episode":    episode,
				"old_path":   filePath,
				"new_path":   fullPath,
				"anime_type": anime.Type,
			},
			Time: time.Now(),
		}
		if volume != nil {
			ev.OpenLibraryDir = volume.volume.openSidecarDir
		}
		o.eventBus.Publish(ev)
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

func savedTemplate(key string) string {
	if database.DB != nil {
		var setting database.Setting
		if err := database.DB.Where("key = ?", key).First(&setting).Error; err == nil {
			return setting.Value
		}
	}
	return ""
}

// selectTemplate 同时区分自动的剧集布局和用户明确保存的模板。
func (o *TVOrganizer) selectTemplate(anime core.Anime) (template string, automaticTV bool) {
	switch normalizeAnimeType(anime.Type) {
	case "movie":
		if saved := savedTemplate("MOVIE_TEMPLATE"); saved != "" {
			return saved, false
		}
		if o.movieTemplate != "" {
			return o.movieTemplate, false
		}
		return fallbackMovieTemplate, false
	case "tv":
		if saved := savedTemplate("TV_TEMPLATE"); saved != "" {
			return saved, false
		}
		if o.tvTemplate == "" {
			return fallbackTVTemplate, false
		}
	default:
		if saved := savedTemplate("OTHER_TEMPLATE"); saved != "" {
			return saved, false
		}
		if o.otherTemplate != "" {
			return o.otherTemplate, false
		}
		if o.tvTemplate == "" {
			return fallbackSpecialTemplate, false
		}
		// 特别篇可以继承启动配置中的 TV 模板。
		if o.tvTemplate == config.DefaultTVTemplate && savedTemplate("TV_TEMPLATE") == o.tvTemplate {
			return o.tvTemplate, false
		}
	}
	return o.tvTemplate, o.tvTemplate == config.DefaultTVTemplate && os.Getenv("TV_TEMPLATE") == ""
}

func (o *TVOrganizer) getBasePath(kind string) string {
	// 常规整理保留原有的启动配置回退；显式补回操作会检查数据库读取错误。
	volume, _ := o.RestoreVolume(kind, database.DB)
	return volume.Root
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

type tvSeriesRoots struct {
	current, legacy string
}

// namingInput 为预览和实际整理构造相同的初始路径，并向钩子保留原始语言元数据。
func (o *TVOrganizer) namingInput(filePath string, anime core.Anime, episode core.Episode) (core.NamingHookInput, tvSeriesRoots) {
	template, automaticTV := o.selectTemplate(anime)
	values := o.buildVarValues(anime, episode)
	values.Ext = filepath.Ext(filePath)
	renderValues := values
	renderLayout := template
	if automaticTV || template == legacyTVTemplate || template == config.DefaultMovieTemplate ||
		template == fallbackTVTemplate || template == fallbackMovieTemplate || template == fallbackSpecialTemplate {
		// 内置目录名优先中文，文件名优先英文。自定义模板中的语言占位符保持原义。
		renderValues.TitleCN = firstTitle(anime.TitleCN, anime.TitleJP, anime.TitleEN)
		renderValues.TitleEN = firstTitle(anime.TitleEN, anime.TitleCN, anime.TitleJP)
		if template == fallbackTVTemplate && resolveVar("tmdb_id", values) == "" {
			renderLayout = strings.Replace(template, " [tmdbid={tmdb_id}]", "", 1)
		}
	}
	var roots tvSeriesRoots
	if automaticTV && anime.Year > 0 {
		rootLayout, _, _ := strings.Cut(template, "/")
		legacyLayout, _, _ := strings.Cut(legacyTVTemplate, "/")
		roots = tvSeriesRoots{current: renderTemplate(rootLayout, renderValues), legacy: renderTemplate(legacyLayout, renderValues)}
	}
	return core.NamingHookInput{
		Anime: anime, Episode: episode, Template: template, VarValues: values,
		RenderedPath: renderTemplate(renderLayout, renderValues),
	}, roots
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
