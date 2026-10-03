package ai

import (
	"testing"
)

func TestCatalogLookup(t *testing.T) {
	cat := DefaultCatalog()

	// Builtin exact lookup
	r1 := cat.Lookup("deepseek-reasoner")
	if !r1.Capabilities.SupportsThinking {
		t.Errorf("expected deepseek-reasoner to support thinking")
	}
	if r1.Provider != ProviderDeepSeek {
		t.Errorf("expected deepseek-reasoner provider to be %s, got %s", ProviderDeepSeek, r1.Provider)
	}

	// Inferred dynamic model (DeepSeek-R1-Distill-Qwen-32B)
	r2 := cat.Lookup("deepseek-r1-distill-qwen-32b")
	if !r2.Capabilities.SupportsThinking {
		t.Errorf("expected dynamic deepseek-r1 to support thinking")
	}

	// Inferred Claude 3.7
	r3 := cat.Lookup("claude-3-7-sonnet")
	if !r3.Capabilities.SupportsThinking {
		t.Errorf("expected claude-3.7 to support thinking")
	}
	if r3.Protocol != ProtocolAnthropic {
		t.Errorf("expected claude-3-7 protocol to be %s, got %s", ProtocolAnthropic, r3.Protocol)
	}

	// Inferred OpenAI o1
	r4 := cat.Lookup("o1-preview")
	if !r4.Capabilities.SupportsReasoningEffort {
		t.Errorf("expected o1 to support reasoning effort")
	}
}

func TestCatalogRegister(t *testing.T) {
	cat := NewCatalog()
	spec := ModelSpec{
		ID:       "custom-local-model",
		Provider: ProviderOllama,
		Protocol: ProtocolOllama,
		Capabilities: Capabilities{
			DefaultMaxTokens: 1024,
		},
	}
	cat.Register(spec)

	found := cat.Lookup("custom-local-model")
	if found.ID != "custom-local-model" || found.Capabilities.DefaultMaxTokens != 1024 {
		t.Errorf("failed to lookup registered custom model: %+v", found)
	}
}
