package search

import (
	"context"
	"fmt"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type mockSource struct{ calls int }

func (m *mockSource) Name() string { return "Mock" }
func (m *mockSource) FetchRSS(ctx context.Context, url string) ([]core.TorrentItem, error) { return nil, nil }
func (m *mockSource) FetchHistory(ctx context.Context, id string, filter core.Filter) ([]core.TorrentItem, error) { return nil, nil }
func (m *mockSource) IsAvailable(ctx context.Context) bool { return true }
func (m *mockSource) SearchAnime(ctx context.Context, title string) ([]core.TorrentItem, error) {
	m.calls++
	return []core.TorrentItem{{Title: title + " Episode 01", SourceName: "Mikan"}}, nil
}

func TestAggregatorDedupesAndPreservesOrder(t *testing.T) {
	items := []core.TorrentItem{
		{Title: "Same Title", SourceName: "Mikan"},
		{Title: "same title", SourceName: "Nyaa"},
		{Title: "Other Title", SourceName: "ACG.RIP"},
	}
	source := &singleSource{items: items}
	got, err := NewAggregator(source).AggregateSearch(context.Background(), []string{"a", "A"}, nil)
	if err != nil || len(got) != 2 || got[0].Title != "Same Title" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestAggregatorCapsAt60(t *testing.T) {
	items := make([]core.TorrentItem, 80)
	for i := range items {
		items[i].Title = fmt.Sprintf("Title %03d", i)
	}
	source := &singleSource{items: items}
	got, err := NewAggregator(source).AggregateSearch(context.Background(), []string{"q"}, nil)
	if err != nil || len(got) != MaxResults {
		t.Fatalf("len=%d err=%v", len(got), err)
	}
}

type singleSource struct{ items []core.TorrentItem }

func (s *singleSource) Name() string { return "Single" }
func (s *singleSource) FetchRSS(ctx context.Context, url string) ([]core.TorrentItem, error) { return nil, nil }
func (s *singleSource) FetchHistory(ctx context.Context, id string, filter core.Filter) ([]core.TorrentItem, error) { return nil, nil }
func (s *singleSource) IsAvailable(ctx context.Context) bool { return true }
func (s *singleSource) SearchAnime(ctx context.Context, title string) ([]core.TorrentItem, error) {
	return s.items, nil
}
