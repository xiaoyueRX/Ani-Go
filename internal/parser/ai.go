package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type aiChatClient interface {
	Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	IsAvailable(ctx context.Context) bool
}

type AIParser struct {
	client aiChatClient
}

func NewAIParser(client aiChatClient) *AIParser {
	return &AIParser{client: client}
}

func (p *AIParser) Name() string { return "ai" }

func (p *AIParser) Parse(ctx context.Context, input string) (core.ParseResult, error) {
	result := core.ParseResult{RawInput: input, Season: 1}

	if p.client == nil || !p.client.IsAvailable(ctx) {
		return result, fmt.Errorf("AI 不可用")
	}

	systemPrompt := `你是一个番剧管理助手，专门解析用户的自然语言指令。
请根据用户输入提取关键字段，返回纯 JSON（不要 markdown 代码块）。
字段: action(subscribe/unsubscribe/list/search), title(番剧名), season(数字), resolution, subgroup_pref, keywords(数组), confidence(0-1)`

	response, err := p.client.Chat(ctx, systemPrompt, buildParsePrompt(input))
	if err != nil {
		return result, fmt.Errorf("AI 解析失败: %w", err)
	}

	if err := json.Unmarshal([]byte(extractJSON(response)), &result); err != nil {
		result.Title = input
		result.Action = "subscribe"
		result.Confidence = 0.3
		return result, nil
	}

	if result.Action == "" {
		result.Action = "subscribe"
	}
	if result.Season == 0 {
		result.Season = 1
	}
	result.RawInput = input

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
