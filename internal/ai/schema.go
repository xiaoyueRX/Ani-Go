package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	jsonBlockRegex = regexp.MustCompile("(?s)```(?:json)?\\s*({.*?}|\\[.*?\\])\\s*```")
	thinkTagRegex  = regexp.MustCompile("(?s)<think>.*?</think>")
)

// CleanResponseText strips thought chains and extracts pure content.
func CleanResponseText(raw string) string {
	cleaned := thinkTagRegex.ReplaceAllString(raw, "")
	return strings.TrimSpace(cleaned)
}

// ExtractJSON attempts to find and return the primary JSON substring from a model response.
func ExtractJSON(text string) string {
	clean := CleanResponseText(text)

	// 1. Try markdown code block
	if matches := jsonBlockRegex.FindStringSubmatch(clean); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	startObj := strings.Index(clean, "{")
	startArr := strings.Index(clean, "[")

	// Determine whether object or array appears first
	if startArr >= 0 && (startObj < 0 || startArr < startObj) {
		lastArr := strings.LastIndex(clean, "]")
		if lastArr > startArr {
			return strings.TrimSpace(clean[startArr : lastArr+1])
		}
	} else if startObj >= 0 {
		lastObj := strings.LastIndex(clean, "}")
		if lastObj > startObj {
			return strings.TrimSpace(clean[startObj : lastObj+1])
		}
	}

	return clean
}

// ParseStructured parses raw model output into target struct type T.
func ParseStructured[T any](raw string) (*T, error) {
	jsonStr := ExtractJSON(raw)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in model response")
	}

	var target T
	if err := json.Unmarshal([]byte(jsonStr), &target); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w (raw: %s)", err, jsonStr)
	}

	return &target, nil
}
