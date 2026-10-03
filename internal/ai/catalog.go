package ai

import (
	"strings"
	"sync"
)

// ProviderType identifies the upstream model provider or protocol family.
type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"
	ProviderAnthropic ProviderType = "anthropic"
	ProviderGemini    ProviderType = "gemini"
	ProviderDeepSeek  ProviderType = "deepseek"
	ProviderOllama    ProviderType = "ollama"
)

// Capabilities defines model-specific features and behavioral traits.
type Capabilities struct {
	// SupportsThinking indicates whether the model generates a reasoning/thinking trace.
	SupportsThinking bool
	// SupportsReasoningEffort indicates whether reasoning_effort (low/medium/high) is accepted.
	SupportsReasoningEffort bool
	// ThinkingBudgetMultiplier is used to elevate max_tokens to prevent output truncation.
	ThinkingBudgetMultiplier float64
	// DefaultMaxTokens is the standard completion token ceiling when not explicitly configured.
	DefaultMaxTokens int
}

// ModelSpec encapsulates metadata and execution properties for an AI model.
type ModelSpec struct {
	ID           string
	Provider     ProviderType
	Protocol     Protocol // ProtocolOpenAI, ProtocolAnthropic, ProtocolGoogle, ProtocolOllama
	Capabilities Capabilities
}

// Catalog holds registered models and dynamic fallback resolvers.
type Catalog struct {
	mu     sync.RWMutex
	models map[string]ModelSpec
}

var (
	defaultCatalogInstance *Catalog
	defaultCatalogOnce     sync.Once
)

// DefaultCatalog returns the global singleton model catalog.
func DefaultCatalog() *Catalog {
	defaultCatalogOnce.Do(func() {
		defaultCatalogInstance = NewCatalog()
		defaultCatalogInstance.registerBuiltinModels()
	})
	return defaultCatalogInstance
}

// NewCatalog initializes an empty model catalog.
func NewCatalog() *Catalog {
	return &Catalog{
		models: make(map[string]ModelSpec),
	}
}

// Register adds or updates a model specification in the catalog.
func (c *Catalog) Register(spec ModelSpec) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.models[strings.ToLower(spec.ID)] = spec
}

// Lookup retrieves a model spec by ID, or infers capabilities for unknown models.
func (c *Catalog) Lookup(modelID string) ModelSpec {
	c.mu.RLock()
	cleanID := strings.ToLower(strings.TrimSpace(modelID))
	if spec, ok := c.models[cleanID]; ok {
		c.mu.RUnlock()
		return spec
	}
	c.mu.RUnlock()

	// Inferred dynamic model spec based on model name patterns (aligned with pi-ai)
	return c.inferSpec(cleanID)
}

func (c *Catalog) inferSpec(modelID string) ModelSpec {
	spec := ModelSpec{
		ID:       modelID,
		Provider: ProviderOpenAI,
		Protocol: ProtocolOpenAI,
		Capabilities: Capabilities{
			DefaultMaxTokens:         2048,
			ThinkingBudgetMultiplier: 1.0,
		},
	}

	// DeepSeek patterns
	if strings.Contains(modelID, "deepseek-r1") || strings.Contains(modelID, "deepseek-reasoner") {
		spec.Provider = ProviderDeepSeek
		spec.Capabilities.SupportsThinking = true
		spec.Capabilities.DefaultMaxTokens = 8192
		spec.Capabilities.ThinkingBudgetMultiplier = 1.5
		return spec
	}
	if strings.Contains(modelID, "deepseek") {
		spec.Provider = ProviderDeepSeek
		spec.Capabilities.DefaultMaxTokens = 4096
		return spec
	}

	// Claude / Anthropic patterns
	if strings.Contains(modelID, "claude") {
		spec.Provider = ProviderAnthropic
		spec.Protocol = ProtocolAnthropic
		spec.Capabilities.DefaultMaxTokens = 4096
		if strings.Contains(modelID, "3-7") || strings.Contains(modelID, "3.7") || strings.Contains(modelID, "thinking") {
			spec.Capabilities.SupportsThinking = true
			spec.Capabilities.DefaultMaxTokens = 8192
			spec.Capabilities.ThinkingBudgetMultiplier = 1.5
		}
		return spec
	}

	// Gemini patterns
	if strings.Contains(modelID, "gemini") {
		spec.Provider = ProviderGemini
		spec.Protocol = ProtocolGoogle
		spec.Capabilities.DefaultMaxTokens = 4096
		if strings.Contains(modelID, "2.5") || strings.Contains(modelID, "2-5") || strings.Contains(modelID, "thinking") {
			spec.Capabilities.SupportsThinking = true
			spec.Capabilities.DefaultMaxTokens = 8192
			spec.Capabilities.ThinkingBudgetMultiplier = 1.5
		}
		return spec
	}

	// OpenAI reasoning models (o1, o3)
	if strings.HasPrefix(modelID, "o1") || strings.HasPrefix(modelID, "o3") {
		spec.Capabilities.SupportsThinking = true
		spec.Capabilities.SupportsReasoningEffort = true
		spec.Capabilities.DefaultMaxTokens = 8192
		spec.Capabilities.ThinkingBudgetMultiplier = 1.5
		return spec
	}

	return spec
}

func (c *Catalog) registerBuiltinModels() {
	builtins := []ModelSpec{
		// DeepSeek
		{
			ID:       "deepseek-chat",
			Provider: ProviderDeepSeek,
			Protocol: ProtocolOpenAI,
			Capabilities: Capabilities{
				DefaultMaxTokens: 4096,
			},
		},
		{
			ID:       "deepseek-reasoner",
			Provider: ProviderDeepSeek,
			Protocol: ProtocolOpenAI,
			Capabilities: Capabilities{
				SupportsThinking:         true,
				DefaultMaxTokens:         8192,
				ThinkingBudgetMultiplier: 1.5,
			},
		},
		// OpenAI
		{
			ID:       "gpt-4o",
			Provider: ProviderOpenAI,
			Protocol: ProtocolOpenAI,
			Capabilities: Capabilities{
				DefaultMaxTokens: 4096,
			},
		},
		{
			ID:       "gpt-4o-mini",
			Provider: ProviderOpenAI,
			Protocol: ProtocolOpenAI,
			Capabilities: Capabilities{
				DefaultMaxTokens: 2048,
			},
		},
		{
			ID:       "o1",
			Provider: ProviderOpenAI,
			Protocol: ProtocolOpenAI,
			Capabilities: Capabilities{
				SupportsThinking:         true,
				SupportsReasoningEffort:  true,
				DefaultMaxTokens:         8192,
				ThinkingBudgetMultiplier: 1.5,
			},
		},
		{
			ID:       "o3-mini",
			Provider: ProviderOpenAI,
			Protocol: ProtocolOpenAI,
			Capabilities: Capabilities{
				SupportsThinking:         true,
				SupportsReasoningEffort:  true,
				DefaultMaxTokens:         8192,
				ThinkingBudgetMultiplier: 1.5,
			},
		},
		// Anthropic
		{
			ID:       "claude-3-5-sonnet-20241022",
			Provider: ProviderAnthropic,
			Protocol: ProtocolAnthropic,
			Capabilities: Capabilities{
				DefaultMaxTokens: 4096,
			},
		},
		{
			ID:       "claude-3-7-sonnet-20250219",
			Provider: ProviderAnthropic,
			Protocol: ProtocolAnthropic,
			Capabilities: Capabilities{
				SupportsThinking:         true,
				DefaultMaxTokens:         8192,
				ThinkingBudgetMultiplier: 1.5,
			},
		},
		// Gemini
		{
			ID:       "gemini-2.0-flash",
			Provider: ProviderGemini,
			Protocol: ProtocolGoogle,
			Capabilities: Capabilities{
				DefaultMaxTokens: 4096,
			},
		},
		{
			ID:       "gemini-2.5-flash",
			Provider: ProviderGemini,
			Protocol: ProtocolGoogle,
			Capabilities: Capabilities{
				SupportsThinking:         true,
				DefaultMaxTokens:         8192,
				ThinkingBudgetMultiplier: 1.5,
			},
		},
	}

	for _, m := range builtins {
		c.Register(m)
	}
}
