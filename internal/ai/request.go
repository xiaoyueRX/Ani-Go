// Package ai 提供 AI 辅助功能（可选模块）
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

	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

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
		httpClient: httpx.New(30 * time.Second),
		endpoint:   endpoint,
		apiKey:     apiKey,
		model:      model,
	}
}

func (b *openAIBackend) isAvailable() bool {
	return b.endpoint != ""
}

func (b *openAIBackend) chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return b.chatWithModel(ctx, b.model, systemPrompt, userPrompt)
}

func (b *openAIBackend) chatWithModel(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	reqBody := openAIRequest{
		Model: model,
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
		httpClient: httpx.New(30 * time.Second),
		apiKey:     apiKey,
		model:      model,
	}
}

func (b *googleBackend) isAvailable() bool {
	return b.apiKey != ""
}

func (b *googleBackend) chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return b.chatWithModel(ctx, b.model, systemPrompt, userPrompt)
}

func (b *googleBackend) chatWithModel(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model, b.apiKey)

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
		httpClient: httpx.New(120 * time.Second),
		host:       host,
		model:      model,
	}
}

func (b *ollamaBackend) isAvailable() bool {
	return b.host != ""
}

func (b *ollamaBackend) chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return b.chatWithModel(ctx, b.model, systemPrompt, userPrompt)
}

func (b *ollamaBackend) chatWithModel(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	url := b.host + "/api/chat"

	reqBody := ollamaRequest{
		Model: model,
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
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	System      string          `json:"system"`
	Messages    []claudeMessage `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
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
		httpClient: httpx.New(30 * time.Second),
		apiKey:     apiKey,
		model:      model,
	}
}

func (b *anthropicBackend) isAvailable() bool {
	return b.apiKey != ""
}

func (b *anthropicBackend) chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return b.chatWithModel(ctx, b.model, systemPrompt, userPrompt)
}

func (b *anthropicBackend) chatWithModel(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	reqBody := claudeRequest{
		Model:     model,
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