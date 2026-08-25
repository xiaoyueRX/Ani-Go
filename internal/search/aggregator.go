package search

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"golang.org/x/sync/semaphore"
)

const (
	MaxResults    = maxResults
	searchWorkers = 4
	totalTimeout  = 30 * time.Second
	maxResults    = 60
)

// Aggregator performs every final search through real configured sources.
type Aggregator struct {
	source core.Source
	sem    *semaphore.Weighted
}

func NewAggregator(source core.Source) *Aggregator {
	return &Aggregator{source: source, sem: semaphore.NewWeighted(searchWorkers)}
}

type aggregateResult struct {
	items []core.TorrentItem
	err   error
}

// AggregateSearch searches candidate terms concurrently through the source,
// normalizes titles for cross-source dedupe, filters sources, and caps at 60.
func (a *Aggregator) AggregateSearch(ctx context.Context, queries []string, allowedSources []string) ([]core.TorrentItem, error) {
	ctx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()

	unique := dedupeStrings(queries)
	results := make(chan aggregateResult, len(unique))
	wg := sync.WaitGroup{}

	go func() {
		for _, query := range unique {
			select {
			case <-ctx.Done():
			default:
			}
			if ctx.Err() != nil {
				break
			}
			if err := a.sem.Acquire(ctx, 1); err != nil {
				break
			}
			wg.Add(1)
			go func(query string) {
				defer wg.Done()
				defer a.sem.Release(1)
				items, err := a.source.SearchAnime(ctx, query)
				results <- aggregateResult{items: items, err: err}
			}(query)
		}
		wg.Wait()
		close(results)
	}()

	seen := make(map[string]struct{})
	sourceFilter := make(map[string]struct{}, len(allowedSources))
	for _, name := range allowedSources {
		sourceFilter[strings.ToLower(name)] = struct{}{}
	}
	items := make([]core.TorrentItem, 0, maxResults)
	var lastErr error
	for result := range results {
		if result.err != nil {
			lastErr = result.err
			continue
		}
		for _, item := range result.items {
			key := normalizeTitle(item.Title)
			if _, ok := seen[key]; ok {
				continue
			}
			if len(sourceFilter) > 0 {
				if _, ok := sourceFilter[strings.ToLower(item.SourceName)]; !ok {
					continue
				}
			}
			seen[key] = struct{}{}
			items = append(items, item)
			if len(items) == maxResults {
				return items, nil
			}
		}
	}
	if len(items) == 0 {
		return items, lastErr
	}
	return items, nil
}

func normalizeTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	return strings.NewReplacer(" ", "", "\t", "", "　", "").Replace(title)
}
