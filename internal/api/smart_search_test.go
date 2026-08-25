package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/search"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

type stubSource struct{ items []core.TorrentItem }

func (s *stubSource) Name() string { return "Stub" }
func (s *stubSource) FetchRSS(ctx context.Context, url string) ([]core.TorrentItem, error) {
	return nil, nil
}
func (s *stubSource) FetchHistory(ctx context.Context, id string, filter core.Filter) ([]core.TorrentItem, error) {
	return nil, nil
}
func (s *stubSource) IsAvailable(ctx context.Context) bool { return true }
func (s *stubSource) SearchAnime(ctx context.Context, title string) ([]core.TorrentItem, error) {
	return s.items, nil
}

type stubAI struct{ err error }

func (s stubAI) Classify(ctx context.Context, title, description string) (*ai.ClassifyResult, error) {
	return nil, nil
}
func (s stubAI) SuggestMerge(ctx context.Context, titles []string) ([]ai.MergeSuggestion, error) {
	return nil, nil
}
func (s stubAI) IsAvailable(ctx context.Context) bool { return true }
func (s stubAI) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return `{"queries":["Frieren"]}`, nil
}

func newSmartServer(t *testing.T, chat stubAI) *Server {
	t.Helper()
	multi := source.NewMultiSource(&stubSource{items: []core.TorrentItem{{Title: "Frieren 01", SourceName: "Mikan"}}})
	return &Server{
		multiSrc:        multi,
		smartAggregator: search.NewAggregator(multi),
		smartExpander:   search.NewExpander(chat, true),
	}
}

func callSmart(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/search/smart", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	srv.handleSmartSearch(w, req)
	return w
}

func TestSmartSearchHandler(t *testing.T) {
	setupTestDB(t)
	srv := newSmartServer(t, stubAI{})

	w := callSmart(t, srv, `{"query":"芙莉莲","limit":100,"offset":0}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got smartSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ExpandedQueries) != 2 || got.ExpandedQueries[1] != "Frieren" || !got.UsedAI {
		t.Fatalf("got=%+v", got)
	}
	if got.Total != 1 || got.HasMore || len(got.Items) != 1 {
		t.Fatalf("pagination=%+v", got)
	}
}

func TestSmartSearchHandlerAIDegraded(t *testing.T) {
	setupTestDB(t)
	srv := newSmartServer(t, stubAI{err: context.DeadlineExceeded})
	w := callSmart(t, srv, `{"query":"芙莉莲"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var got smartSearchResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.UsedAI || got.AIError != "timeout" || len(got.Items) != 1 {
		t.Fatalf("got=%+v", got)
	}
}

func TestSmartSearchHandlerUnauthorized(t *testing.T) {
	mux := http.NewServeMux()
	s := newSmartServer(t, stubAI{})
	s.registerRoutes(mux)
	handler := auth.AuthMiddleware(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/search/smart", strings.NewReader(`{"query":"x"}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestSmartSearchHandlerValidation(t *testing.T) {
	setupTestDB(t)
	srv := newSmartServer(t, stubAI{})
	if code := callSmart(t, srv, `{"query":"  "}`).Code; code != http.StatusBadRequest {
		t.Fatalf("empty query status=%d", code)
	}
	long := strings.Repeat("a", 201)
	if code := callSmart(t, srv, `{"query":"`+long+`"}`).Code; code != http.StatusBadRequest {
		t.Fatalf("long query status=%d", code)
	}
	var got smartSearchResponse
	w := callSmart(t, srv, `{"query":"q","limit":999}`)
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Items) != 1 {
		t.Fatalf("clamped limit response=%+v", got)
	}
}

func BenchmarkSmartSearch(b *testing.B) {
	items := make([]core.TorrentItem, 100)
	for i := range items {
		items[i].Title = "Frieren Episode " + strings.Repeat("x", 20) + string(rune('a'+i%26)) + string(rune('0'+i%10))
	}
	multi := source.NewMultiSource(&stubSource{items: items})
	srv := &Server{
		multiSrc:        multi,
		smartAggregator: search.NewAggregator(multi),
		smartExpander:   search.NewExpander(stubAI{}, false),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/search/smart", strings.NewReader(`{"query":"Frieren","limit":20}`))
		srv.handleSmartSearch(w, req)
		if w.Code != http.StatusOK {
			b.Fatal(w.Code)
		}
	}
}
