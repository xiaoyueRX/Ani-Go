// Package search provides AI-assisted query expansion and multi-source aggregation.
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
)

const (
	maxExpandConcurrency = 3
	expandTimeout        = 10 * time.Second
	circuitFailureLimit  = 5
	circuitOpenDuration  = 30 * time.Second
	maxQueries           = 6
)

// Errors classify AI degradation for API consumers.
var (
	ErrAITimeout = errors.New("timeout")
	ErrAIParse   = errors.New("parse_failed")
)

var queryArrayRe = regexp.MustCompile(`(?s)"queries"\s*:\s*\[(.*?)\]`)
var quotedStringRe = regexp.MustCompile(`"((?:\\.|[^"\\])*)"`)

type aiChatClient interface {
	Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Expander turns a colloquial anime title into candidate source-search terms.
// AI output is treated strictly as suggestions; callers perform real searches.
type Expander struct {
	client    aiChatClient
	enabled   bool
	sem       *semaphore.Weighted
	mu        sync.Mutex
	failures  int
	openUntil time.Time
}

func NewExpander(client aiChatClient, enabled bool) *Expander {
	return &Expander{client: client, enabled: enabled, sem: semaphore.NewWeighted(maxExpandConcurrency)}
}

// Expand returns candidate queries. It never returns download links.
func (e *Expander) Expand(ctx context.Context, input string) ([]string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return []string{input}, nil
	}
	if !e.enabled || e.client == nil || e.CircuitOpen() {
		return []string{input}, nil
	}

	if err := e.sem.Acquire(ctx, 1); err != nil {
		return []string{input}, err
	}
	defer e.sem.Release(1)

	callCtx, cancel := context.WithTimeout(ctx, expandTimeout)
	defer cancel()

	systemPrompt := `你是番剧搜索词生成器。给定用户的中文/口语化番剧名，输出 JSON：{"queries":["日文原名","英文官方名","罗马音","常见简称"]}。只依据真实存在的动画作品，禁止编造；最多6个词；按最可能命中的排序；只返回严格JSON。`
	response, err := e.client.Chat(callCtx, systemPrompt, input)
	if err != nil {
		e.recordFailure()
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return []string{input}, fmt.Errorf("%w: %v", ErrAITimeout, err)
		}
		return []string{input}, err
	}

	queries := parseQueries(response)
	if len(queries) == 0 {
		e.recordFailure()
		return []string{input}, fmt.Errorf("%w: empty or invalid response", ErrAIParse)
	}
	e.recordSuccess()
	return dedupeStrings(append([]string{input}, queries...)), nil
}

// CircuitOpen reports whether repeated AI failures temporarily suppress AI.
func (e *Expander) CircuitOpen() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.openUntil.After(time.Now())
}

func (e *Expander) recordSuccess() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failures = 0
	e.openUntil = time.Time{}
}

func (e *Expander) recordFailure() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failures++
	if e.failures >= circuitFailureLimit {
		e.openUntil = time.Now().Add(circuitOpenDuration)
		e.failures = 0
	}
}

func parseQueries(response string) []string {
	var payload struct {
		Queries []string `json:"queries"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(response)), &payload); err == nil {
		return cleanQueries(payload.Queries)
	}
	if match := queryArrayRe.FindStringSubmatch(response); match != nil {
		var values []string
		if err := json.Unmarshal([]byte("["+match[1]+"]"), &values); err == nil {
			return cleanQueries(values)
		}
		for _, raw := range quotedStringRe.FindAllStringSubmatch(match[1], -1) {
			value, _ := jsonUnquote(raw[1])
			values = append(values, value)
		}
		return cleanQueries(values)
	}
	return nil
}

func cleanQueries(values []string) []string {
	limit := len(values)
	if limit > maxQueries {
		limit = maxQueries
	}
	cleaned := make([]string, 0, limit)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
		if len(cleaned) == maxQueries {
			break
		}
	}
	return dedupeStrings(cleaned)
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func jsonUnquote(value string) (string, error) {
	var decoded string
	err := json.Unmarshal([]byte(`"`+value+`"`), &decoded)
	return decoded, err
}
