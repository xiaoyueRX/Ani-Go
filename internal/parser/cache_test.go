package parser

import (
	"path/filepath"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func TestCachedTitleResult(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_cache.db")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init failed: %v", err)
	}

	raw := "[Sakurato] Steel Ball Run - 01 [1080p HEVC]"

	// First query: should be miss
	if _, ok := GetCachedTitleResult(raw); ok {
		t.Fatalf("expected cache miss, got hit")
	}

	// Save
	if err := SaveCachedTitleResult(raw, "Steel Ball Run", 1, 1.0, "Sakurato", "1080p", 0.95, "regex"); err != nil {
		t.Fatalf("SaveCachedTitleResult failed: %v", err)
	}

	// Second query: should hit
	res, ok := GetCachedTitleResult(raw)
	if !ok {
		t.Fatalf("expected cache hit, got miss")
	}
	if res.Season != 1 || res.Episode != 1.0 || res.Subgroup != "Sakurato" || res.Resolution != "1080p" {
		t.Errorf("unexpected cached result: %+v", res)
	}
	if res.ResolvedBy != "regex" {
		t.Errorf("unexpected resolvedBy: %s", res.ResolvedBy)
	}
}
