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
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

// NormalizeBaseURL cleans and normalizes OpenAI-compatible base endpoints.
// If the path does not end with /chat/completions, it appends it or /v1/chat/completions appropriately.
func NormalizeBaseURL(rawURL string) string {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return "https://api.openai.com/v1/chat/completions"
	}
	u = strings.TrimRight(u, "/")
	if strings.HasSuffix(u, "/chat/completions") {
		return u
	}
	if strings.HasSuffix(u, "/v1") {
		return u + "/chat/completions"
	}
	return u + "/v1/chat/completions"
}

// ============================================================
// OpenAI 兼容后端（支持所有 /v1/chat/completions 端点及自愈降级）
// ============================================================

type openAIBackend struct {
	httpClient     *http.Client
	endpoint       string
	apiKey         string
	model          string
	mu             sync.RWMutex
	extrasDisabled bool // Aligned with pi-ai _extras_disabled: suppresses reasoning_effort/thinking on 400
}

type openAIRequest struct {
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
	Temperature     float64       `json:"temperature"`
	MaxTokens       int           `json:"max_tokens"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
}

type openAIChoice struct {
	Message struct {
		Role             string `json:"role"`
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content,omitempty"`
	} `json:"message"`
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
}

func newOpenAIBackend(endpoint, apiKey, model string) *openAIBackend {
	return &openAIBackend{
		httpClient: httpx.New(60 * time.Second),
		endpoint:   NormalizeBaseURL(endpoint),
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
	spec := DefaultCatalog().Lookup(model)

	b.mu.RLock()
	extrasOff := b.extrasDisabled
	b.mu.RUnlock()

	maxTokens := spec.Capabilities.DefaultMaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}

	reqBody := openAIRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
		MaxTokens:   maxTokens,
	}

	if !extrasOff && spec.Capabilities.SupportsReasoningEffort {
		reqBody.ReasoningEffort = "low"
	}

	res, err := b.doPost(ctx, reqBody)
	if err != nil {
		// If 400 Bad Request happens and extras are enabled, retry once without extras
		if !extrasOff && strings.Contains(err.Error(), "400") {
			log.Printf("⚠️ OpenAI/中转网关返回 400，自动触发 pi-ai extrasDisabled 静默降级并重试...")
			b.mu.Lock()
			b.extrasDisabled = true
			b.mu.Unlock()

			reqBody.ReasoningEffort = ""
			reqBody.MaxTokens = 2048
			return b.doPost(ctx, reqBody)
		}
		return "", err
	}
	return res, nil
}

func (b *openAIBackend) doPost(ctx context.Context, reqBody openAIRequest) (string, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化 AI 请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建 AI 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if b.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.apiKey)
		// Dual handshake for NewAPI/relay gateways
		req.Header.Set("api-key", b.apiKey)
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

	choice := cr.Choices[0]
	if choice.Message.ReasoningContent != "" {
		log.Printf("🧠 [Thinking Trace]: %s", strings.TrimSpace(choice.Message.ReasoningContent)[:min(150, len(choice.Message.ReasoningContent))])
	}

	content := CleanResponseText(choice.Message.Content)
	log.Printf("🤖 AI 响应: %s", content[:min(200, len(content))])
	return content, nil
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
		httpClient: httpx.New(45 * time.Second),
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

	content := CleanResponseText(parts[0].Text)
	log.Printf("🤖 Google 响应: %s", content[:min(200, len(content))])
	return content, nil
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

	content := CleanResponseText(or.Message.Content)
	log.Printf("🤖 Ollama 响应: %s", content[:min(200, len(content))])
	return content, nil
}

// ============================================================
// Anthropic Claude 后端（支持双认证握手与 Thinking 自适应）
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

type claudeThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type claudeRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	System      string          `json:"system"`
	Messages    []claudeMessage `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
	Thinking    *claudeThinking `json:"thinking,omitempty"`
}

type claudeResponseContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`
}

type claudeResponse struct {
	Content []claudeResponseContent `json:"content"`
}

func newAnthropicBackend(apiKey, model string) *anthropicBackend {
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	return &anthropicBackend{
		httpClient: httpx.New(60 * time.Second),
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
	spec := DefaultCatalog().Lookup(model)
	maxTokens := spec.Capabilities.DefaultMaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}

	reqBody := claudeRequest{
		Model:     model,
		MaxTokens: maxTokens,
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
	// Dual auth handshake (both x-api-key and Bearer for official & NewAPI proxies)
	req.Header.Set("x-api-key", b.apiKey)
	req.Header.Set("Authorization", "Bearer "+b.apiKey)
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

	var resultText string
	for _, block := range cr.Content {
		if block.Thinking != "" {
			log.Printf("🧠 [Claude Thinking]: %s", strings.TrimSpace(block.Thinking)[:min(150, len(block.Thinking))])
		}
		if block.Text != "" {
			resultText += block.Text
		}
	}

	if resultText == "" {
		return "", fmt.Errorf("Anthropic 未返回任何文本内容")
	}

	content := CleanResponseText(resultText)
	log.Printf("🤖 Anthropic 响应: %s", content[:min(200, len(content))])
	return content, nil
}
