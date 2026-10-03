package ai

import (
	"testing"
)

func TestCleanResponseText(t *testing.T) {
	raw := "<think>\nThis is an internal reasoning trace from DeepSeek-R1 or Claude.\nWe think step by step.\n</think>\n{\"type\":\"anime\",\"confidence\":0.95}"

	cleaned := CleanResponseText(raw)
	expected := "{\"type\":\"anime\",\"confidence\":0.95}"
	if cleaned != expected {
		t.Errorf("expected %s, got %s", expected, cleaned)
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "markdown json block",
			input:    "Here is your classification:\n```json\n{\"category\":\"TV\"}\n```\nHope it helps!",
			expected: "{\"category\":\"TV\"}",
		},
		{
			name:     "embedded raw json object",
			input:    "Classification output: {\"success\": true} finished.",
			expected: "{\"success\": true}",
		},
		{
			name:     "embedded raw json array",
			input:    "Results: [{\"id\": 1}, {\"id\": 2}] end.",
			expected: "[{\"id\": 1}, {\"id\": 2}]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := ExtractJSON(tt.input)
			if res != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, res)
			}
		})
	}
}

func TestParseStructured(t *testing.T) {
	type TestPayload struct {
		Title  string `json:"title"`
		Season int    `json:"season"`
	}

	raw := "<think>thinking...</think> ```json\n{\"title\": \"Frieren\", \"season\": 1}\n```"
	res, err := ParseStructured[TestPayload](raw)
	if err != nil {
		t.Fatalf("unexpected error parsing structured json: %v", err)
	}

	if res.Title != "Frieren" || res.Season != 1 {
		t.Errorf("unexpected parsed struct: %+v", res)
	}
}
