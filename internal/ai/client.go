// Package ai 提供 AI 辅助功能（可选模块）
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// ============================================================
// Client 统一 AI 客户端
// ============================================================

type Client struct {
	backend     aiBackend
	model       string
	backupModel string
}

// NewClient 创建 AI 客户端，自动检测协议
// endpoint 为空时尝试从环境变量推断
func NewClient(endpoint, apiKey, model string) *Client {
	return NewClientWithProtocol(endpoint, apiKey, model, ProtocolAuto)
}

// NewClientWithProtocol 创建指定协议的 AI 客户端
func NewClientWithProtocol(endpoint, apiKey, model string, proto Protocol) *Client {
	return newClient(endpoint, apiKey, model, "", proto)
}

// NewClientWithBackup 创建支持备用模型的 AI 客户端
func NewClientWithBackup(endpoint, apiKey, model, backupModel string, proto Protocol) *Client {
	return newClient(endpoint, apiKey, model, backupModel, proto)
}

func newClient(endpoint, apiKey, model, backupModel string, proto Protocol) *Client {
	if model == "" {
		model = "gpt-4o-mini"
	}

	if proto == ProtocolAuto || proto == "" {
		proto = detectProtocol(endpoint)
	}

	log.Printf("🤖 AI 协议: %s | 模型: %s", proto, model)

	var backend aiBackend
	switch proto {
	case ProtocolGoogle:
		backend = newGoogleBackend(apiKey, model)
	case ProtocolAnthropic:
		backend = newAnthropicBackend(apiKey, model)
	case ProtocolOllama:
		backend = newOllamaBackend(endpoint, model)
	default:
		backend = newOpenAIBackend(endpoint, apiKey, model)
	}

	if backupModel != "" && backupModel != model {
		log.Printf("🛟 AI 备用模型: %s", backupModel)
	}
	return &Client{backend: backend, model: model, backupModel: backupModel}
}

// detectProtocol 根据端点自动检测协议类型
func detectProtocol(endpoint string) Protocol {
	lower := strings.ToLower(endpoint)
	if strings.Contains(lower, "generativelanguage.googleapis.com") {
		return ProtocolGoogle
	}
	if strings.Contains(lower, "anthropic.com") {
		return ProtocolAnthropic
	}
	if strings.Contains(lower, "ollama") || strings.Contains(lower, ":11434") {
		return ProtocolOllama
	}
	return ProtocolOpenAI
}

// IsAvailable 检查 AI 服务是否可用
func (c *Client) IsAvailable(ctx context.Context) bool {
	return c.backend != nil && c.backend.isAvailable()
}

// Chat 通用对话接口，发送自定义系统提示和用户提示，返回模型原始响应
// 内置容灾：最多2次重试(指数退避) + 主模型失败自动切换备用模型
func (c *Client) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.backend == nil {
		return "", fmt.Errorf("AI 后端未初始化")
	}

	const maxRetries = 2
	backoff := []time.Duration{100 * time.Millisecond, 300 * time.Millisecond}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("🔁 AI 请求重试 (%d/%d): %v", attempt, maxRetries, lastErr)
			select {
			case <-time.After(backoff[attempt-1]):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		resp, err := c.backend.chatWithModel(ctx, c.model, systemPrompt, userPrompt)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}

	// 主模型全部失败 → 尝试备用模型
	if c.backupModel != "" && c.backupModel != c.model {
		log.Printf("⚠️ 主模型 %s 失败(%v)，切换备用模型 %s", c.model, lastErr, c.backupModel)
		resp, err := c.backend.chatWithModel(ctx, c.backupModel, systemPrompt, userPrompt)
		if err == nil {
			return resp, nil
		}
		return "", fmt.Errorf("主模型与备用模型均失败: 主=%v, 备=%w", lastErr, err)
	}
	return "", lastErr
}

// Classify 使用 AI 对番剧进行分类
func (c *Client) Classify(ctx context.Context, title, description string) (*ClassifyResult, error) {
	userPrompt := fmt.Sprintf(`你是一个动漫分类专家。请根据以下番剧信息判断其类型。

番剧名: %s
描述: %s

请返回 JSON 格式：
{"type": "TV|Movie|OVA|Special", "confidence": 0.0-1.0, "reason": "分类依据"}

分类规则：
- TV: 电视动画连续剧
- Movie: 剧场版/动画电影（时长 > 60 分钟）
- OVA: OVA/OAD/番外篇
- Special: 特别篇/特典/SP

只返回 JSON，不要其他文字。`, title, description)

	systemPrompt := "你是一个专业的动漫数据分类助手。始终返回严格的 JSON 格式，不要包含任何额外解释或 markdown 标记。"

	result, err := c.backend.chat(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	return parseClassifyResult(result)
}

// SuggestMerge 使用 AI 建议将多个番剧归并为同一系列
func (c *Client) SuggestMerge(ctx context.Context, titles []string) ([]MergeSuggestion, error) {
	namesJSON, _ := json.Marshal(titles)
	userPrompt := fmt.Sprintf(`你是一个动漫元数据专家。以下是一些番剧名称列表，请分析哪些可能属于同一系列。

番剧列表: %s

请返回 JSON 数组格式，将可能属于同一系列的番剧归组：
[{"group_name": "系列名", "anime_ids": ["番剧名1", "番剧名2"], "reason": "归并依据"}]

只返回 JSON 数组，不要其他文字。`, string(namesJSON))

	systemPrompt := "你是一个专业的动漫数据分类助手。始终返回严格的 JSON 格式，不要包含任何额外解释或 markdown 标记。"

	result, err := c.backend.chat(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	return parseMergeResult(result)
}

// ============================================================
// JSON 解析辅助
// ============================================================

func parseClassifyResult(raw string) (*ClassifyResult, error) {
	var cr ClassifyResult
	if err := json.Unmarshal([]byte(raw), &cr); err != nil {
		if start := strings.Index(raw, "{"); start >= 0 {
			if end := strings.LastIndex(raw, "}"); end > start {
				if err := json.Unmarshal([]byte(raw[start:end+1]), &cr); err != nil {
					return nil, fmt.Errorf("AI 分类结果解析失败: %w", err)
				}
				return &cr, nil
			}
		}
		return nil, fmt.Errorf("AI 分类结果解析失败: %w", err)
	}
	return &cr, nil
}

func parseMergeResult(raw string) ([]MergeSuggestion, error) {
	var suggestions []MergeSuggestion
	if err := json.Unmarshal([]byte(raw), &suggestions); err != nil {
		if start := strings.Index(raw, "["); start >= 0 {
			if end := strings.LastIndex(raw, "]"); end > start {
				if err := json.Unmarshal([]byte(raw[start:end+1]), &suggestions); err != nil {
					return nil, fmt.Errorf("AI 归并建议解析失败: %w", err)
				}
				return suggestions, nil
			}
		}
		return nil, fmt.Errorf("AI 归并建议解析失败: %w", err)
	}
	return suggestions, nil
}

// ============================================================
// 通用
// ============================================================

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}