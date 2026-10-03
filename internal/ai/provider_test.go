package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/v1/", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/chat/completions"},
		{"https://api.deepseek.com", "https://api.deepseek.com/v1/chat/completions"},
		{"https://my-relay.com/", "https://my-relay.com/v1/chat/completions"},
	}

	for _, c := range cases {
		got := NormalizeBaseURL(c.input)
		if got != c.expected {
			t.Errorf("NormalizeBaseURL(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestOpenAIBackendFallbackOn400(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		var req openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// First attempt with reasoning_effort returns 400 Bad Request (simulating NewAPI unknown parameter error)
		if attempt == 1 && req.ReasoningEffort != "" {
			http.Error(w, `{"error":{"message":"unknown parameter reasoning_effort"}}`, http.StatusBadRequest)
			return
		}

		// Second attempt without reasoning_effort succeeds
		w.Header().Set("Content-Type", "application/json")
		resp := openAIResponse{
			Choices: []openAIChoice{
				{
					Message: struct {
						Role             string `json:"role"`
						Content          string `json:"content"`
						ReasoningContent string `json:"reasoning_content,omitempty"`
					}{
						Role:             "assistant",
						Content:          `{"status": "ok"}`,
						ReasoningContent: "thinking trace",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	backend := newOpenAIBackend(server.URL, "test-key", "o1-mini")
	res, err := backend.chat(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("expected successful fallback, got err: %v", err)
	}

	if res != `{"status": "ok"}` {
		t.Errorf("unexpected content: %s", res)
	}

	if attempt != 2 {
		t.Errorf("expected 2 attempts for 400 fallback, got %d", attempt)
	}

	if !backend.extrasDisabled {
		t.Errorf("expected extrasDisabled to be set to true")
	}
}

func TestAnthropicDualAuthHandshake(t *testing.T) {
	var gotXAPIKey, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXAPIKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")

		w.Header().Set("Content-Type", "application/json")
		resp := claudeResponse{
			Content: []claudeResponseContent{
				{Type: "text", Text: `{"result": "success"}`},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	backend := newAnthropicBackend("secret-key", "claude-3-5-sonnet")
	// inject custom test client with server url override by creating request directly or testing headers
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-api-key", backend.apiKey)
	req.Header.Set("Authorization", "Bearer "+backend.apiKey)

	client := server.Client()
	_, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if gotXAPIKey != "secret-key" || gotAuth != "Bearer secret-key" {
		t.Errorf("unexpected auth headers: x-api-key=%q, Authorization=%q", gotXAPIKey, gotAuth)
	}
}
