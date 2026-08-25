package search

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type mockChatClient struct {
	response string
	err      error
	delay    time.Duration
	calls    int
}

func (m *mockChatClient) Chat(ctx context.Context, _, _ string) (string, error) {
	m.calls++
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return m.response, m.err
}

func TestExpanderValidJSON(t *testing.T) {
	client := &mockChatClient{response: `{"queries":["葬送のフリーレン","Frieren","Frieren"]}`}
	got, err := NewExpander(client, true).Expand(context.Background(), "芙莉莲")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"芙莉莲", "葬送のフリーレン", "Frieren"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestExpanderDirtyOutput(t *testing.T) {
	client := &mockChatClient{response: "```json\n前缀 {\"queries\": [\"Frieren\", \"フリーレン\"]} 后缀\n```"}
	got, err := NewExpander(client, true).Expand(context.Background(), "芙莉莲")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1] != "Frieren" || got[2] != "フリーレン" {
		t.Fatalf("got %v", got)
	}
}

func TestExpanderInvalidResponse(t *testing.T) {
	client := &mockChatClient{response: "not json"}
	got, err := NewExpander(client, true).Expand(context.Background(), "芙莉莲")
	if err == nil || !errors.Is(err, ErrAIParse) {
		t.Fatalf("err=%v", err)
	}
	if len(got) != 1 || got[0] != "芙莉莲" {
		t.Fatalf("got %v", got)
	}
}

func TestExpanderTimeout(t *testing.T) {
	client := &mockChatClient{delay: 20 * time.Millisecond}
	expander := NewExpander(client, true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	got, err := expander.Expand(ctx, "芙莉莲")
	if err == nil || !errors.Is(err, ErrAITimeout) {
		t.Fatalf("err=%v", err)
	}
	if len(got) != 1 || got[0] != "芙莉莲" {
		t.Fatalf("got %v", got)
	}
}

func TestExpanderDisabled(t *testing.T) {
	client := &mockChatClient{}
	got, _ := NewExpander(client, false).Expand(context.Background(), "芙莉莲")
	if client.calls != 0 || len(got) != 1 || got[0] != "芙莉莲" {
		t.Fatalf("calls=%d got=%v", client.calls, got)
	}
}

func TestExpanderCircuitOpensAfterFiveFailures(t *testing.T) {
	client := &mockChatClient{response: ""}
	expander := NewExpander(client, true)
	for range 5 {
		expander.Expand(context.Background(), "query")
	}
	if !expander.CircuitOpen() {
		t.Fatal("circuit did not open")
	}
	before := client.calls
	expander.Expand(context.Background(), "query")
	if client.calls != before {
		t.Fatal("AI called while circuit open")
	}
}
