// Package ai 提供 AI 辅助功能（可选模块）
// 支持 OpenAI / Google / Anthropic / Ollama 及所有 OpenAI 兼容协议（DeepSeek、Groq 等）
// 关闭 AI 后所有核心功能不受影响
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ============================================================
// 公开类型
// ============================================================

type AnimeType string

const (
	TypeTV      AnimeType = "TV"
	TypeMovie   AnimeType = "Movie"
	TypeOVA     AnimeType = "OVA"
	TypeSpecial AnimeType = "Special"
)

// Protocol AI 服务商协议类型
type Protocol string

const (
	ProtocolAuto     Protocol = ""          // 自动检测
	ProtocolOpenAI   Protocol = "openai"    // OpenAI 及兼容协议（DeepSeek/Groq/通义千问/智谱等）
	ProtocolGoogle   Protocol = "google"    // Google Gemini
	ProtocolAnthropic Protocol = "anthropic" // Anthropic Claude
	ProtocolOllama   Protocol = "ollama"    // Ollama 本地部署
)

type ClassifyResult struct {
	Type       AnimeType `json:"type"`
	Confidence float64   `json:"confidence"`
	Reason     string    `json:"reason"`
}

type MergeSuggestion struct {
	GroupName string   `json:"group_name"`
	AnimeIDs  []string `json:"anime_ids"`
	Reason    string   `json:"reason"`
}

// Classifier AI 分类器接口
type Classifier interface {
	Classify(ctx context.Context, title, description string) (*ClassifyResult, error)
	SuggestMerge(ctx context.Context, titles []string) ([]MergeSuggestion, error)
	Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	IsAvailable(ctx context.Context) bool
}

// ============================================================
// 内部后端接口
// ============================================================

type aiBackend interface {
	chat(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	isAvailable() bool
}

// ============================================================
// 通用 chatMessage 结构（OpenAI 兼容格式）
// ============================================================

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ============================================================
// Client 统一 AI 客户端
// ============================================================

type Client struct {
	backend aiBackend
	model   string
}

// NewClient 创建 AI 客户端，自动检测协议
// endpoint 为空时尝试从环境变量推断
func NewClient(endpoint, apiKey, model string) *Client {
	return NewClientWithProtocol(endpoint, apiKey, model, ProtocolAuto)
}

// NewClientWithProtocol 创建指定协议的 AI 客户端
func NewClientWithProtocol(endpoint, apiKey, model string, proto Protocol) *Client {
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

	return &Client{backend: backend, model: model}
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
func (c *Client) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.backend == nil {
		return "", fmt.Errorf("AI 后端未初始化")
	}
	return c.backend.chat(ctx, systemPrompt, userPrompt)
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
// OpenAI 兼容后端（支持所有 /v1/chat/completions 端点）
// ============================================================

type openAIBackend struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
	model      string
}

type openAIRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type openAIResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func newOpenAIBackend(endpoint, apiKey, model string) *openAIBackend {
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	}
	return &openAIBackend{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		endpoint:   endpoint,
		apiKey:     apiKey,
		model:      model,
	}
}

func (b *openAIBackend) isAvailable() bool {
	return b.endpoint != ""
}

func (b *openAIBackend) chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := openAIRequest{
		Model: b.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
		MaxTokens:   1024,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建 AI 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if b.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.apiKey)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AI API 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	var cr openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", fmt.Errorf("AI 响应解析失败: %w", err)
	}

	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("AI 未返回任何内容")
	}

	content := cr.Choices[0].Message.Content
	log.Printf("🤖 AI 响应: %s", strings.TrimSpace(content)[:min(200, len(content))])
	return strings.TrimSpace(content), nil
}

// ============================================================
// Google Gemini 后端
// ============================================================

type googleBackend struct {
	httpClient *http.Client
	apiKey     string
	model      string
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
}

func newGoogleBackend(apiKey, model string) *googleBackend {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &googleBackend{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiKey:     apiKey,
		model:      model,
	}
}

func (b *googleBackend) isAvailable() bool {
	return b.apiKey != ""
}

func (b *googleBackend) chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		b.model, b.apiKey)

	contents := []geminiContent{
		{Parts: []geminiPart{{Text: systemPrompt + "\n\n" + userPrompt}}},
	}

	bodyBytes, _ := json.Marshal(geminiRequest{Contents: contents})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建 Google 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Google 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Google API 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	var gr geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return "", fmt.Errorf("Google 响应解析失败: %w", err)
	}

	if len(gr.Candidates) == 0 {
		return "", fmt.Errorf("Google 未返回任何内容")
	}

	parts := gr.Candidates[0].Content.Parts
	if len(parts) == 0 {
		return "", fmt.Errorf("Google 响应为空")
	}

	content := parts[0].Text
	log.Printf("🤖 Google 响应: %s", strings.TrimSpace(content)[:min(200, len(content))])
	return strings.TrimSpace(content), nil
}

// ============================================================
// Ollama 后端（本地部署）
// ============================================================

type ollamaBackend struct {
	httpClient *http.Client
	host       string
	model      string
}

type ollamaRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ollamaResponse struct {
	Message chatMessage `json:"message"`
}

func newOllamaBackend(host, model string) *ollamaBackend {
	if host == "" {
		host = "http://localhost:11434"
	}
	host = strings.TrimSuffix(host, "/")
	if model == "" {
		model = "llama3"
	}
	return &ollamaBackend{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		host:       host,
		model:      model,
	}
}

func (b *ollamaBackend) isAvailable() bool {
	return b.host != ""
}

func (b *ollamaBackend) chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	url := b.host + "/api/chat"

	reqBody := ollamaRequest{
		Model: b.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建 Ollama 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Ollama 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama API 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	var or ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return "", fmt.Errorf("Ollama 响应解析失败: %w", err)
	}

	content := or.Message.Content
	log.Printf("🤖 Ollama 响应: %s", strings.TrimSpace(content)[:min(200, len(content))])
	return strings.TrimSpace(content), nil
}

// ============================================================
// Anthropic Claude 后端
// ============================================================

type anthropicBackend struct {
	httpClient *http.Client
	apiKey     string
	model      string
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeRequest struct {
	Model       string         `json:"model"`
	MaxTokens   int            `json:"max_tokens"`
	System      string         `json:"system"`
	Messages    []claudeMessage `json:"messages"`
	Temperature float64        `json:"temperature,omitempty"`
}

type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func newAnthropicBackend(apiKey, model string) *anthropicBackend {
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	return &anthropicBackend{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiKey:     apiKey,
		model:      model,
	}
}

func (b *anthropicBackend) isAvailable() bool {
	return b.apiKey != ""
}

func (b *anthropicBackend) chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := claudeRequest{
		Model:     b.model,
		MaxTokens: 1024,
		System:    systemPrompt,
		Messages: []claudeMessage{
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建 Anthropic 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", b.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Anthropic 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Anthropic API 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	var cr claudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", fmt.Errorf("Anthropic 响应解析失败: %w", err)
	}

	if len(cr.Content) == 0 {
		return "", fmt.Errorf("Anthropic 未返回任何内容")
	}

	content := cr.Content[0].Text
	log.Printf("🤖 Anthropic 响应: %s", strings.TrimSpace(content)[:min(200, len(content))])
	return strings.TrimSpace(content), nil
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
