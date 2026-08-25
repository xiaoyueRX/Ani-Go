package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type inMemoryServer struct {
	handler  http.Handler
	requests []string
}

func (server *inMemoryServer) RoundTrip(request *http.Request) (*http.Response, error) {
	var payload struct {
		Model string `json:"model"`
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(nil)
	server.requests = append(server.requests, payload.Model)

	if payload.Model == "primary" {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}

	recorder := httptest.NewRecorder()
	server.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

func TestClientChatFallsBackToBackupModel(t *testing.T) {
	server := &inMemoryServer{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"role": "assistant", "content": "backup-ok"}},
				},
			})
		}),
	}

	client := NewClientWithBackup("http://ai.test/v1/chat/completions", "", "primary", "backup", ProtocolOpenAI)
	backend := client.backend.(*openAIBackend)
	backend.httpClient = &http.Client{Transport: server}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	response, err := client.Chat(ctx, "system", "user")
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if response != "backup-ok" {
		t.Fatalf("Chat() = %q, want %q", response, "backup-ok")
	}

	want := []string{"primary", "primary", "primary", "backup"}
	if len(server.requests) != len(want) {
		t.Fatalf("model requests = %v, want %v", server.requests, want)
	}
	for i := range want {
		if server.requests[i] != want[i] {
			t.Fatalf("model requests = %v, want %v", server.requests, want)
		}
	}
}
