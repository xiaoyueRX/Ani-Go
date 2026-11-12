package parser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

const (
	// defaultAIDailyLimit 默认每日 AI 解析调用上限（30 次）
	defaultAIDailyLimit = 30
	// maxAIInputRunes 单次送入 AI 的最大输入字符数，防止超长 Prompt 注入与 Token 浪费
	maxAIInputRunes = 512
)

var (
	// ErrAIQuotaExceeded 每日 AI 配额耗尽错误
	ErrAIQuotaExceeded = errors.New("ai parser daily quota exceeded")
)

type aiChatClient interface {
	Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	IsAvailable(ctx context.Context) bool
}

// DailyQuotaGuard 管理 AI 每日调用限额与零点重置（并发安全）
type DailyQuotaGuard struct {
	mu           sync.RWMutex
	dailyLimit   int
	currentCount int
	currentDate  string // YYYY-MM-DD
}

// NewDailyQuotaGuard 创建每日调用限额守护器
func NewDailyQuotaGuard(dailyLimit int) *DailyQuotaGuard {
	if dailyLimit <= 0 {
		dailyLimit = defaultAIDailyLimit
	}
	return &DailyQuotaGuard{
		dailyLimit:  dailyLimit,
		currentDate: time.Now().Format("2006-01-02"),
	}
}

// SetDailyLimit 设置每日调用上限
func (g *DailyQuotaGuard) SetDailyLimit(limit int) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if limit > 0 {
		g.dailyLimit = limit
	}
}

// GetDailyLimit 获取每日调用上限
func (g *DailyQuotaGuard) GetDailyLimit() int {
	if g == nil {
		return 0
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.dailyLimit
}

// GetCurrentCount 获取今日已调用次数
func (g *DailyQuotaGuard) GetCurrentCount() int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checkDateReset()
	return g.currentCount
}

// Reset 手动重置今日计数（主要用于测试与运维）
func (g *DailyQuotaGuard) Reset() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.currentCount = 0
	g.currentDate = time.Now().Format("2006-01-02")
}

// CheckAndIncrement 检查并自增配额计数；若超额则拒绝并返回 false
func (g *DailyQuotaGuard) CheckAndIncrement() (int, bool) {
	if g == nil {
		return 0, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checkDateReset()

	if g.currentCount >= g.dailyLimit {
		return g.currentCount, false
	}

	g.currentCount++
	return g.currentCount, true
}

func (g *DailyQuotaGuard) checkDateReset() {
	today := time.Now().Format("2006-01-02")
	if g.currentDate != today {
		g.currentDate = today
		g.currentCount = 0
	}
}

type AIParser struct {
	client     aiChatClient
	quotaGuard *DailyQuotaGuard
}

func NewAIParser(client aiChatClient) *AIParser {
	return &AIParser{
		client:     client,
		quotaGuard: NewDailyQuotaGuard(defaultAIDailyLimit),
	}
}

// NewAIParserWithQuota 创建附带自定义配额守护器的 AIParser
func NewAIParserWithQuota(client aiChatClient, quotaGuard *DailyQuotaGuard) *AIParser {
	if quotaGuard == nil {
		quotaGuard = NewDailyQuotaGuard(defaultAIDailyLimit)
	}
	return &AIParser{
		client:     client,
		quotaGuard: quotaGuard,
	}
}

func (p *AIParser) Name() string { return "ai" }

// SetDailyLimit 设置每日调用上限
func (p *AIParser) SetDailyLimit(limit int) {
	if p != nil && p.quotaGuard != nil {
		p.quotaGuard.SetDailyLimit(limit)
	}
}

// GetDailyLimit 获取每日调用上限
func (p *AIParser) GetDailyLimit() int {
	if p == nil || p.quotaGuard == nil {
		return 0
	}
	return p.quotaGuard.GetDailyLimit()
}

// GetCurrentCount 获取今日已调用次数
func (p *AIParser) GetCurrentCount() int {
	if p == nil || p.quotaGuard == nil {
		return 0
	}
	return p.quotaGuard.GetCurrentCount()
}

// QuotaGuard 获取底层的每日配额守护器
func (p *AIParser) QuotaGuard() *DailyQuotaGuard {
	if p == nil {
		return nil
	}
	return p.quotaGuard
}

type aiParseResponse struct {
	Action       string   `json:"action"`
	Title        string   `json:"title"`
	Season       int      `json:"season"`
	Resolution   string   `json:"resolution"`
	SubgroupPref string   `json:"subgroup_pref"`
	Keywords     []string `json:"keywords"`
	Confidence   float64  `json:"confidence"`
}

func (p *AIParser) Parse(ctx context.Context, input string) (core.ParseResult, error) {
	trimmedInput := strings.TrimSpace(input)
	result := core.ParseResult{RawInput: trimmedInput, Season: 1, Confidence: 0.0}

	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	if p == nil || p.client == nil || !p.client.IsAvailable(ctx) {
		return result, fmt.Errorf("AI 不可用")
	}

	// 严格控制单次送入大模型的字符上限并清洗控制字符，防止提示词注入
	safeInput := sanitizeControlRunes(trimmedInput)
	safeInput = truncateRunes(safeInput, maxAIInputRunes)
	safeInput = strings.TrimSpace(safeInput)
	if safeInput == "" {
		result.Action = "subscribe"
		return result, nil
	}

	// 每日配额守卫拦截
	if p.quotaGuard != nil {
		count, ok := p.quotaGuard.CheckAndIncrement()
		if !ok {
			return result, fmt.Errorf("AI 每日调用次数已达上限 (%d/%d): %w", count, p.quotaGuard.GetDailyLimit(), ErrAIQuotaExceeded)
		}
	}

	systemPrompt := `你是一个番剧管理助手，专门解析用户的自然语言指令。
请根据用户输入提取关键字段，返回纯 JSON（不要 markdown 代码块）。
字段: action(subscribe/unsubscribe/list/search), title(番剧名), season(数字), resolution, subgroup_pref, keywords(数组), confidence(0-1)`

	response, err := p.client.Chat(ctx, systemPrompt, buildParsePrompt(safeInput))
	if err != nil {
		return result, fmt.Errorf("AI 解析失败: %w", err)
	}

	jsonStr := extractJSON(response)
	var resp aiParseResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		result.Title = safeInput
		result.Action = "subscribe"
		result.Confidence = 0.2
		return result, nil
	}

	// 校验并清洗各字段，杜绝非法季号、集数与异常浮点值
	result.Title = cleanTitle(resp.Title)
	result.SubgroupPref = strings.TrimSpace(resp.SubgroupPref)
	result.Resolution = strings.ToLower(strings.TrimSpace(resp.Resolution))
	result.Keywords = resp.Keywords

	// 规范化 Action
	switch strings.ToLower(strings.TrimSpace(resp.Action)) {
	case "subscribe", "sub", "追番", "订阅":
		result.Action = "subscribe"
	case "unsubscribe", "unsub", "取消订阅", "退订":
		result.Action = "unsubscribe"
	case "list", "ls", "查看", "列出":
		result.Action = "list"
	case "search", "find", "搜索", "查":
		result.Action = "search"
	default:
		result.Action = "subscribe"
	}

	// 规范化 Season (防止大模型输出负数或极大数值)
	if resp.Season < 0 || resp.Season > maxSeasonNumber {
		result.Season = 1
	} else if resp.Season == 0 && result.Action != "list" {
		result.Season = 1
	} else {
		result.Season = resp.Season
	}

	// 规范化 Confidence (防止 NaN/Inf 与越界)
	if math.IsNaN(resp.Confidence) || math.IsInf(resp.Confidence, 0) || resp.Confidence < 0 {
		result.Confidence = 0.0
	} else if resp.Confidence > 1.0 {
		result.Confidence = 1.0
	} else {
		result.Confidence = resp.Confidence
	}

	// 防毒策略：若大模型提取的标题为空且非查看列表指令，置信度降至 0.1
	if result.Title == "" && result.Action != "list" {
		result.Title = safeInput
		result.Confidence = 0.1
	}

	return result, nil
}

func buildParsePrompt(input string) string {
	return fmt.Sprintf(`解析以下番剧管理指令，返回纯 JSON（不要 markdown 代码块）：
输入: "%s"

字段说明：
- action: subscribe(追番/订阅), unsubscribe(取消订阅/退订), list(查看/列出), search(搜索)
- title: 番剧名称（去除季数、字幕组、分辨率后的纯标题）
- season: 季号，数字，未指定默认 1
- resolution: 如 1080p, 720p，无则为空
- subgroup_pref: 字幕组偏好，无则为空
- keywords: 额外关键词数组
- confidence: 0-1 置信度

示例输出：
{"action":"subscribe","title":"某科学的超电磁炮","season":1,"resolution":"","subgroup_pref":"澄空","keywords":[],"confidence":0.9}

请严格返回单行 JSON。`, input)
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```JSON")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
