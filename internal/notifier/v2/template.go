package v2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// NotifyTemplatesSettingKey 保存在 database.Setting 表中的通知模板配置键名
const NotifyTemplatesSettingKey = "NOTIFY_TEMPLATES_JSON"

// defaultChannelKey 内部表示事件级默认模板（JSON 序列化时映射为 "default"）
const defaultChannelKey = "default"

// TemplateContext 结构化通知模板上下文数据
type TemplateContext struct {
	Title           string         `json:"title"`
	Name            string         `json:"name"`
	Content         string         `json:"content"`
	EventType       string         `json:"event_type"`
	Channel         string         `json:"channel"`
	AnimeTitle      string         `json:"anime_title"`
	Episode         any            `json:"episode"`
	Season          int            `json:"season"`
	Size            string         `json:"size"`
	RawSize         int64          `json:"raw_size"`
	SavePath        string         `json:"save_path"`
	NewPath         string         `json:"new_path"`
	Error           string         `json:"error"`
	Source          string         `json:"source"`
	Timestamp       string         `json:"timestamp"`
	Duration        string         `json:"duration"`
	URL             string         `json:"url"`
	Hash            string         `json:"hash"`
	Success         int            `json:"success"`
	Failed          int            `json:"failed"`
	NewCount        int            `json:"new_count"`
	TotalEpisodes   int            `json:"total_episodes"`
	CurrentEpisodes int            `json:"current_episodes"`
	Extra           map[string]any `json:"extra,omitempty"`
}

// ToMap 将结构化上下文转换为模板执行所需的 map[string]any
func (tc TemplateContext) ToMap() map[string]any {
	m := map[string]any{
		"Title":           tc.Title,
		"Name":            tc.Name,
		"Content":         tc.Content,
		"EventType":       tc.EventType,
		"Channel":         tc.Channel,
		"AnimeTitle":      tc.AnimeTitle,
		"Episode":         tc.Episode,
		"Season":          tc.Season,
		"Size":            tc.Size,
		"RawSize":         tc.RawSize,
		"SavePath":        tc.SavePath,
		"NewPath":         tc.NewPath,
		"Error":           tc.Error,
		"Source":          tc.Source,
		"Timestamp":       tc.Timestamp,
		"Duration":        tc.Duration,
		"URL":             tc.URL,
		"Hash":            tc.Hash,
		"Success":         tc.Success,
		"Failed":          tc.Failed,
		"NewCount":        tc.NewCount,
		"TotalEpisodes":   tc.TotalEpisodes,
		"CurrentEpisodes": tc.CurrentEpisodes,
	}
	for k, v := range tc.Extra {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	return m
}

// TemplateInfo 单个通知模板条目（供 API 与前端展示）
type TemplateInfo struct {
	EventType string `json:"event_type"`
	Channel   string `json:"channel"`
	Template  string `json:"template"`
	IsCustom  bool   `json:"is_custom"`
}

// TemplateCatalog 通知模板全量目录（含内置默认模板与自定义覆盖）
type TemplateCatalog struct {
	Defaults map[string]string            `json:"defaults"`
	Custom   map[string]map[string]string `json:"custom"`
	Items    []TemplateInfo               `json:"items"`
}

type parsedTemplate struct {
	raw  string
	tmpl *template.Template
}

// TemplateManager 线程安全的通知模板管理器，支持按事件类型与渠道粒度覆盖并持久化到数据库
type TemplateManager struct {
	mu       sync.RWMutex
	defaults map[string]*parsedTemplate            // eventType -> 内置默认模板
	custom   map[string]map[string]*parsedTemplate // eventType -> channel("" 表示事件级默认) -> 自定义模板
	db       *gorm.DB                              // 可选指定 DB；为 nil 时回退到全局 database.DB
}

// NewTemplateManager 初始化通知模板管理器并预装内置默认模板
func NewTemplateManager() *TemplateManager {
	tm := &TemplateManager{
		defaults: make(map[string]*parsedTemplate),
		custom:   make(map[string]map[string]*parsedTemplate),
	}
	tm.registerDefaultTemplates()
	return tm
}

// NewTemplateManagerWithDB 使用指定数据库初始化模板管理器并自动加载自定义模板
func NewTemplateManagerWithDB(db *gorm.DB) *TemplateManager {
	tm := NewTemplateManager()
	tm.db = db
	if err := tm.LoadFromDB(); err != nil {
		log.Printf("⚠️ 加载自定义通知模板失败: %v", err)
	}
	return tm
}

// SetDB 绑定数据库实例（传 nil 表示回退全局 database.DB）
func (tm *TemplateManager) SetDB(db *gorm.DB) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.db = db
}

func (tm *TemplateManager) activeDB() *gorm.DB {
	if tm.db != nil {
		return tm.db
	}
	return database.DB
}

func (tm *TemplateManager) registerDefaultTemplates() {
	builtins := map[string]string{
		core.EventDownloadStarted:     "📥 开始下载：{{.Name | default .Title}}",
		core.EventDownloadCompleted:   "🎉 下载完成：{{.Name | default .AnimeTitle}}\n💾 大小：{{.Size | default \"未知\"}}\n📂 路径：{{.SavePath | default \"默认目录\"}}",
		core.EventDownloadFailed:      "❌ 下载失败：{{.Name | default .Title}}\n🚨 错误：{{.Error | default .Content}}",
		core.EventFileOrganized:       "📂 番剧归档完成：{{.AnimeTitle | default .Title}}\n🎞️ 剧集：第 {{.Episode | default 0}} 集\n📍 新路径：{{.NewPath}}",
		core.EventSubscriptionAdded:   "➕ 新增订阅：{{.Title | default .AnimeTitle}}\n🔍 来源：{{.Source | default \"未知\"}}",
		core.EventSupplementCompleted: "📥 历史剧集补全：{{.AnimeTitle | default .Title}}\n➕ 新增任务：{{.NewCount | default 0}} 集",
		"episode.missing":             "📥 历史剧集补全：{{.AnimeTitle | default .Title}}\n➕ 新增任务：{{.NewCount | default 0}} 集",
		"error":                       "🚨 系统错误：{{.Error | default .Content}}",
	}
	for evType, raw := range builtins {
		t, err := parseSafeTemplate(evType, raw)
		if err != nil {
			log.Printf("⚠️ 注册内置默认模板 [%s] 失败: %v", evType, err)
			continue
		}
		tm.defaults[evType] = &parsedTemplate{raw: raw, tmpl: t}
	}
}

// templateFuncs 返回内置安全模板函数表
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"default":  tmplDefault,
		"upper":    tmplUpper,
		"lower":    tmplLower,
		"truncate": tmplTruncate,
	}
}

// tmplDefault 当值缺失、为空字符串、零值或 nil 时返回默认值（适配管道写法 {{.X | default "y"}}）
func tmplDefault(defVal, val any) any {
	if isEmptyValue(val) {
		return defVal
	}
	return val
}

func isEmptyValue(val any) bool {
	if val == nil {
		return true
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v) == "" || v == "<no value>"
	case bool:
		return !v
	case int:
		return v == 0
	case int64:
		return v == 0
	case float32:
		return v == 0
	case float64:
		return v == 0
	default:
		s := fmt.Sprintf("%v", val)
		return s == "" || s == "<nil>" || s == "<no value>"
	}
}

func toTemplateString(val any) string {
	if val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		if s == "<no value>" {
			return ""
		}
		return s
	}
	s := fmt.Sprintf("%v", val)
	if s == "<nil>" || s == "<no value>" {
		return ""
	}
	return s
}

func tmplUpper(val any) string {
	return strings.ToUpper(toTemplateString(val))
}

func tmplLower(val any) string {
	return strings.ToLower(toTemplateString(val))
}

// tmplTruncate 按 Unicode 字符数安全截断文本，兼容管道写法 {{.Title | truncate 20}} 与函数写法 {{truncate .Title 20}}
func tmplTruncate(arg1, arg2 any) string {
	var maxLen int
	var text string

	switch v := arg1.(type) {
	case int:
		maxLen = v
		text = toTemplateString(arg2)
	case int64:
		maxLen = int(v)
		text = toTemplateString(arg2)
	default:
		text = toTemplateString(arg1)
		switch l := arg2.(type) {
		case int:
			maxLen = l
		case int64:
			maxLen = int(l)
		}
	}

	if maxLen <= 0 || text == "" {
		return ""
	}
	if utf8.RuneCountInString(text) <= maxLen {
		return text
	}
	runes := []rune(text)
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

func parseSafeTemplate(name, raw string) (*template.Template, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("模板内容不能为空")
	}
	t, err := template.New(name).Funcs(templateFuncs()).Option("missingkey=zero").Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("模板语法错误: %w", err)
	}
	return t, nil
}

// normalizeChannel 规范化渠道名：空字符串或 "default" 统一归为 ""（表示事件级默认模板）
func normalizeChannel(channel string) string {
	trimmed := strings.TrimSpace(channel)
	if trimmed == "" || strings.EqualFold(trimmed, defaultChannelKey) {
		return ""
	}
	return trimmed
}

// displayChannel 将内部渠道键转换为对外展示/序列化键名
func displayChannel(channel string) string {
	if channel == "" {
		return defaultChannelKey
	}
	return channel
}

// channelAliases 返回渠道的常见别名集合（小写），支持中英文名称及别名匹配
func channelAliases(channel string) []string {
	low := strings.ToLower(strings.TrimSpace(channel))
	if low == "" {
		return nil
	}
	aliasGroups := [][]string{
		{"钉钉", "dingtalk"},
		{"企业微信", "wecom"},
		{"飞书", "feishu"},
		{"qq(onebot)", "onebot", "qq"},
		{"email", "smtp"},
		{"wechat", "weixin", "微信公众号"},
	}
	for _, group := range aliasGroups {
		for _, item := range group {
			if low == item {
				return group
			}
		}
	}
	return []string{low}
}

// ValidateTemplate 校验模板语法是否合法
func ValidateTemplate(tmpl string) error {
	_, err := parseSafeTemplate("validate", tmpl)
	return err
}

// ValidateTemplate 校验模板语法是否合法（实例方法，供 API 直接调用）
func (tm *TemplateManager) ValidateTemplate(tmpl string) error {
	return ValidateTemplate(tmpl)
}

// Add 安全添加或更新事件级自定义模板（不再使用 template.Must panic）
func (tm *TemplateManager) Add(id string, text string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("模板标识不能为空")
	}
	t, err := parseSafeTemplate(id, text)
	if err != nil {
		return err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.custom[id] == nil {
		tm.custom[id] = make(map[string]*parsedTemplate)
	}
	tm.custom[id][""] = &parsedTemplate{raw: text, tmpl: t}
	return nil
}

// SetTemplate 设置并持久化指定事件与渠道的自定义模板（channel 为空或 "default" 时表示该事件的默认模板）
func (tm *TemplateManager) SetTemplate(eventType, channel, tmplStr string) error {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return fmt.Errorf("事件类型不能为空")
	}
	normCh := normalizeChannel(channel)
	parsed, err := parseSafeTemplate(eventType+":"+displayChannel(normCh), tmplStr)
	if err != nil {
		return err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.custom[eventType] == nil {
		tm.custom[eventType] = make(map[string]*parsedTemplate)
	}
	prev, hadPrev := tm.custom[eventType][normCh]
	tm.custom[eventType][normCh] = &parsedTemplate{raw: tmplStr, tmpl: parsed}

	if err := tm.saveLocked(); err != nil {
		// 回滚内存状态，保证内存与数据库一致
		if hadPrev {
			tm.custom[eventType][normCh] = prev
		} else {
			delete(tm.custom[eventType], normCh)
			if len(tm.custom[eventType]) == 0 {
				delete(tm.custom, eventType)
			}
		}
		return err
	}
	return nil
}

// DeleteTemplate 删除指定事件与渠道的自定义模板并持久化（回退到上级或内置默认模板）
func (tm *TemplateManager) DeleteTemplate(eventType, channel string) error {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return fmt.Errorf("事件类型不能为空")
	}
	normCh := normalizeChannel(channel)

	tm.mu.Lock()
	defer tm.mu.Unlock()

	chMap, ok := tm.custom[eventType]
	if !ok {
		return nil
	}
	prev, hadPrev := chMap[normCh]
	if !hadPrev {
		return nil
	}

	delete(chMap, normCh)
	if len(chMap) == 0 {
		delete(tm.custom, eventType)
	}

	if err := tm.saveLocked(); err != nil {
		// 回滚内存状态
		if tm.custom[eventType] == nil {
			tm.custom[eventType] = make(map[string]*parsedTemplate)
		}
		tm.custom[eventType][normCh] = prev
		return err
	}
	return nil
}

// ListTemplates 返回内置默认模板与所有自定义模板目录
func (tm *TemplateManager) ListTemplates() TemplateCatalog {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	catalog := TemplateCatalog{
		Defaults: make(map[string]string, len(tm.defaults)),
		Custom:   make(map[string]map[string]string, len(tm.custom)),
	}

	defaultKeys := make([]string, 0, len(tm.defaults))
	for k, pt := range tm.defaults {
		catalog.Defaults[k] = pt.raw
		defaultKeys = append(defaultKeys, k)
	}
	sort.Strings(defaultKeys)
	for _, k := range defaultKeys {
		catalog.Items = append(catalog.Items, TemplateInfo{
			EventType: k,
			Channel:   defaultChannelKey,
			Template:  catalog.Defaults[k],
			IsCustom:  false,
		})
	}

	customEvents := make([]string, 0, len(tm.custom))
	for ev := range tm.custom {
		customEvents = append(customEvents, ev)
	}
	sort.Strings(customEvents)
	for _, ev := range customEvents {
		chMap := tm.custom[ev]
		if len(chMap) == 0 {
			continue
		}
		outCh := make(map[string]string, len(chMap))
		chNames := make([]string, 0, len(chMap))
		for ch, pt := range chMap {
			disp := displayChannel(ch)
			outCh[disp] = pt.raw
			chNames = append(chNames, disp)
		}
		catalog.Custom[ev] = outCh
		sort.Strings(chNames)
		for _, disp := range chNames {
			catalog.Items = append(catalog.Items, TemplateInfo{
				EventType: ev,
				Channel:   disp,
				Template:  outCh[disp],
				IsCustom:  true,
			})
		}
	}

	return catalog
}

// LoadFromDB 从 database.Setting (NOTIFY_TEMPLATES_JSON) 加载自定义模板
func (tm *TemplateManager) LoadFromDB() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	db := tm.activeDB()
	if db == nil {
		return nil
	}

	var setting database.Setting
	err := db.Where("key = ?", NotifyTemplatesSettingKey).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			tm.custom = make(map[string]map[string]*parsedTemplate)
			return nil
		}
		return fmt.Errorf("读取通知模板配置失败: %w", err)
	}

	if strings.TrimSpace(setting.Value) == "" {
		tm.custom = make(map[string]map[string]*parsedTemplate)
		return nil
	}

	var rawMap map[string]map[string]string
	if err := json.Unmarshal([]byte(setting.Value), &rawMap); err != nil {
		return fmt.Errorf("解析 NOTIFY_TEMPLATES_JSON 失败: %w", err)
	}

	loaded := make(map[string]map[string]*parsedTemplate)
	var parseErrs []string
	for evType, chMap := range rawMap {
		evType = strings.TrimSpace(evType)
		if evType == "" || len(chMap) == 0 {
			continue
		}
		for ch, tmplStr := range chMap {
			normCh := normalizeChannel(ch)
			parsed, err := parseSafeTemplate(evType+":"+displayChannel(normCh), tmplStr)
			if err != nil {
				parseErrs = append(parseErrs, fmt.Sprintf("%s/%s: %v", evType, displayChannel(normCh), err))
				continue
			}
			if loaded[evType] == nil {
				loaded[evType] = make(map[string]*parsedTemplate)
			}
			loaded[evType][normCh] = &parsedTemplate{raw: tmplStr, tmpl: parsed}
		}
	}

	tm.custom = loaded
	if len(parseErrs) > 0 {
		return fmt.Errorf("部分自定义模板解析失败并已跳过: %s", strings.Join(parseErrs, "; "))
	}
	return nil
}

// SaveToDB 将当前全部自定义模板以 JSON 格式持久化到 database.Setting
func (tm *TemplateManager) SaveToDB() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.activeDB() == nil {
		return fmt.Errorf("数据库未初始化")
	}
	return tm.saveLocked()
}

func (tm *TemplateManager) saveLocked() error {
	db := tm.activeDB()
	if db == nil {
		// 纯内存/无数据库测试环境下跳过持久化
		return nil
	}

	rawMap := make(map[string]map[string]string, len(tm.custom))
	for evType, chMap := range tm.custom {
		if len(chMap) == 0 {
			continue
		}
		out := make(map[string]string, len(chMap))
		for ch, pt := range chMap {
			out[displayChannel(ch)] = pt.raw
		}
		rawMap[evType] = out
	}

	payload, err := json.Marshal(rawMap)
	if err != nil {
		return fmt.Errorf("序列化通知模板配置失败: %w", err)
	}

	setting := database.Setting{
		Key:   NotifyTemplatesSettingKey,
		Value: string(payload),
	}
	if err := db.Save(&setting).Error; err != nil {
		return fmt.Errorf("保存通知模板配置失败: %w", err)
	}
	return nil
}

// Render 渲染指定事件默认模板（保持向后兼容）
func (tm *TemplateManager) Render(templateID string, data interface{}) (string, error) {
	return tm.RenderForChannel(templateID, "", data)
}

// RenderForChannel 按 (eventType, channel) -> (eventType, "") -> 内置默认模板顺序查找并渲染
func (tm *TemplateManager) RenderForChannel(eventType, channel string, data interface{}) (string, error) {
	tm.mu.RLock()
	candidates := tm.lookupCandidatesLocked(eventType, channel, true)
	tm.mu.RUnlock()

	if len(candidates) == 0 {
		return "", fmt.Errorf("未找到模板: %s", eventType)
	}

	execData := prepareTemplateData(eventType, channel, data)
	var lastErr error
	for _, pt := range candidates {
		rendered, err := executeParsedTemplate(pt.tmpl, execData)
		if err == nil {
			return rendered, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("模板渲染错误: %w", lastErr)
}

// RenderCustomForChannel 仅在存在自定义模板时渲染；返回 (渲染结果, 是否命中自定义模板, 错误)
func (tm *TemplateManager) RenderCustomForChannel(eventType, channel string, data interface{}) (string, bool, error) {
	tm.mu.RLock()
	candidates := tm.lookupCandidatesLocked(eventType, channel, false)
	tm.mu.RUnlock()

	if len(candidates) == 0 {
		return "", false, nil
	}

	execData := prepareTemplateData(eventType, channel, data)
	var lastErr error
	for _, pt := range candidates {
		rendered, err := executeParsedTemplate(pt.tmpl, execData)
		if err == nil {
			return rendered, true, nil
		}
		lastErr = err
	}
	return "", true, fmt.Errorf("自定义模板渲染错误: %w", lastErr)
}

// HasCustomTemplate 判断指定事件或渠道是否配置了自定义模板
func (tm *TemplateManager) HasCustomTemplate(eventType, channel string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.lookupCandidatesLocked(eventType, channel, false)) > 0
}

// lookupCandidatesLocked 按继承优先级返回候选模板列表（调用前需持有读锁）
func (tm *TemplateManager) lookupCandidatesLocked(eventType, channel string, includeBuiltin bool) []*parsedTemplate {
	var candidates []*parsedTemplate
	seen := make(map[*parsedTemplate]struct{})
	addUnique := func(pt *parsedTemplate) {
		if pt == nil {
			return
		}
		if _, ok := seen[pt]; ok {
			return
		}
		seen[pt] = struct{}{}
		candidates = append(candidates, pt)
	}

	normCh := normalizeChannel(channel)
	if chMap, ok := tm.custom[eventType]; ok {
		// 1. 精确匹配 (eventType, channel)
		if normCh != "" {
			if pt, ok := chMap[normCh]; ok {
				addUnique(pt)
			}
			// 2. 大小写无关及常见渠道别名匹配
			aliases := channelAliases(normCh)
			for k, pt := range chMap {
				if k == "" {
					continue
				}
				if strings.EqualFold(k, normCh) {
					addUnique(pt)
					continue
				}
				kLow := strings.ToLower(k)
				for _, alias := range aliases {
					if kLow == alias {
						addUnique(pt)
						break
					}
				}
			}
		}
		// 3. 回退到事件级自定义默认模板 (eventType, "")
		if pt, ok := chMap[""]; ok {
			addUnique(pt)
		}
	}

	// 4. 回退到内置默认模板
	if includeBuiltin {
		if pt, ok := tm.defaults[eventType]; ok {
			addUnique(pt)
		}
	}
	return candidates
}

// PreviewTemplate 预览指定模板渲染效果；若 tmpl 为空则预览当前生效模板，若 sampleData 为空则填充默认示例数据
func (tm *TemplateManager) PreviewTemplate(eventType, channel, tmplStr string, sampleData map[string]any) (string, error) {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" && strings.TrimSpace(tmplStr) == "" {
		return "", fmt.Errorf("事件类型和模板内容不能同时为空")
	}

	merged := buildSampleData(eventType, channel)
	for k, v := range sampleData {
		merged[k] = v
	}
	if channel != "" {
		merged["Channel"] = channel
	}
	if eventType != "" {
		merged["EventType"] = eventType
	}

	if strings.TrimSpace(tmplStr) != "" {
		parsed, err := parseSafeTemplate("preview", tmplStr)
		if err != nil {
			return "", err
		}
		return executeParsedTemplate(parsed, merged)
	}

	return tm.RenderForChannel(eventType, channel, merged)
}

func executeParsedTemplate(tmpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func prepareTemplateData(eventType, channel string, data any) any {
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	switch v := data.(type) {
	case nil:
		return map[string]any{
			"EventType": eventType,
			"Channel":   channel,
			"Timestamp": nowStr,
		}
	case TemplateContext:
		if v.EventType == "" {
			v.EventType = eventType
		}
		if channel != "" {
			v.Channel = channel
		}
		if v.Timestamp == "" {
			v.Timestamp = nowStr
		}
		return v.ToMap()
	case *TemplateContext:
		if v == nil {
			return map[string]any{
				"EventType": eventType,
				"Channel":   channel,
				"Timestamp": nowStr,
			}
		}
		cp := *v
		if cp.EventType == "" {
			cp.EventType = eventType
		}
		if channel != "" {
			cp.Channel = channel
		}
		if cp.Timestamp == "" {
			cp.Timestamp = nowStr
		}
		return cp.ToMap()
	case map[string]any:
		cloned := make(map[string]any, len(v)+4)
		for k, val := range v {
			cloned[k] = val
		}
		if _, ok := cloned["EventType"]; !ok {
			cloned["EventType"] = eventType
		}
		if channel != "" {
			cloned["Channel"] = channel
		} else if _, ok := cloned["Channel"]; !ok {
			cloned["Channel"] = ""
		}
		if ts, ok := cloned["Timestamp"]; !ok || isEmptyValue(ts) {
			cloned["Timestamp"] = nowStr
		}
		return cloned
	default:
		// 兼容外部直接传入的自定义结构体
		return data
	}
}

func buildSampleData(eventType, channel string) map[string]any {
	sample := map[string]any{
		"Title":           "Ani-Go 示例通知",
		"Name":            "[ANi] 葬送的芙莉莲 - 01 [1080P][Baha][WEB-DL][AAC AVC][CHT].mp4",
		"Content":         "示例通知内容",
		"EventType":       eventType,
		"Channel":         channel,
		"AnimeTitle":      "葬送的芙莉莲",
		"Episode":         1,
		"Season":          1,
		"Size":            "520.0 MB",
		"RawSize":         int64(545259520),
		"SavePath":        "/downloads/Frieren",
		"NewPath":         "/media/Anime/葬送的芙莉莲/Season 01/葬送的芙莉莲 - S01E01.mkv",
		"Error":           "连接超时 (timeout)",
		"Source":          "Mikan Project",
		"Duration":        "1m12s",
		"Success":         1,
		"Failed":          0,
		"NewCount":        2,
		"TotalEpisodes":   28,
		"CurrentEpisodes": 12,
		"Timestamp":       time.Now().Format("2006-01-02 15:04:05"),
	}
	return sample
}
