// Package ai 提供 AI 辅助功能（可选模块）
// 支持 OpenAI / Google / Anthropic / Ollama 及所有 OpenAI 兼容协议（DeepSeek、Groq 等）
// 关闭 AI 后所有核心功能不受影响
package ai

import (
	"context"
	"errors"
)

// ============================================================
// 公开类型
// ============================================================

var ErrQuotaExceeded = errors.New("quota exceeded")

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
	ProtocolAuto      Protocol = ""          // 自动检测
	ProtocolOpenAI    Protocol = "openai"    // OpenAI 及兼容协议（DeepSeek/Groq/通义千问/智谱等）
	ProtocolGoogle    Protocol = "google"    // Google Gemini
	ProtocolAnthropic Protocol = "anthropic" // Anthropic Claude
	ProtocolOllama    Protocol = "ollama"    // Ollama 本地部署
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
	chatWithModel(ctx context.Context, model, systemPrompt, userPrompt string) (string, error)
	isAvailable() bool
}

// ============================================================
// 通用 chatMessage 结构（OpenAI 兼容格式）
// ============================================================

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}