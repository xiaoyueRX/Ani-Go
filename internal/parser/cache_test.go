package parser

import (
	"path/filepath"
	"strings"
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

// TestParserCache_PoisoningPrevention 测试防毒策略（拒绝缓存零置信度与解析失败结果）
func TestParserCache_PoisoningPrevention(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "poison_test.db")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init failed: %v", err)
	}

	// 1. 低置信度条目（< 0.2）不应被写入缓存
	rawLow := "[Unknown] Badly Formatted Title"
	_ = SaveCachedTitleResult(rawLow, "Badly Formatted Title", 1, 1.0, "", "", 0.1, "ai")
	if _, ok := GetCachedTitleResult(rawLow); ok {
		t.Errorf("低置信度条目不应被持久化缓存")
	}

	// 2. 空标题且集数 <= 0 的条目不应被写入缓存
	rawEmpty := "[Empty]   "
	_ = SaveCachedTitleResult(rawEmpty, "", 1, 0, "", "", 0.8, "regex")
	if _, ok := GetCachedTitleResult(rawEmpty); ok {
		t.Errorf("空标题且集数为0条目不应被缓存")
	}

	// 3. 非法负数季号或集数不应被写入缓存
	rawNeg := "[Bad] Negative Season Title"
	_ = SaveCachedTitleResult(rawNeg, "Negative Season Title", -1, 1.0, "", "", 0.9, "regex")
	if _, ok := GetCachedTitleResult(rawNeg); ok {
		t.Errorf("负数季号条目不应被缓存")
	}
}

// TestParserCache_UpdateAndClear 测试缓存条目更新与批量清空
func TestParserCache_UpdateAndClear(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "update_test.db")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init failed: %v", err)
	}

	raw := "[Subs] Updating Anime Title - 01"

	// 首次缓存为 regex
	_ = SaveCachedTitleResult(raw, "Updating Anime Title", 1, 1.0, "Subs", "720p", 0.7, "regex")
	res1, ok := GetCachedTitleResult(raw)
	if !ok || res1.Resolution != "720p" || res1.ResolvedBy != "regex" {
		t.Fatalf("首次缓存异常: %+v", res1)
	}

	// 二次更新为 airdate 高置信度校准
	_ = SaveCachedTitleResult(raw, "Updating Anime Title", 2, 1.0, "Subs", "1080p", 0.98, "airdate")
	res2, ok := GetCachedTitleResult(raw)
	if !ok || res2.Season != 2 || res2.Resolution != "1080p" || res2.ResolvedBy != "airdate" {
		t.Fatalf("更新缓存异常: %+v", res2)
	}

	// 单条删除
	if err := DeleteCachedTitleResult(raw); err != nil {
		t.Fatalf("DeleteCachedTitleResult 失败: %v", err)
	}
	if _, ok := GetCachedTitleResult(raw); ok {
		t.Errorf("删除后不应再命中缓存")
	}

	// 清空缓存
	_ = SaveCachedTitleResult("Title 1", "Clean 1", 1, 1.0, "", "", 0.9, "regex")
	_ = SaveCachedTitleResult("Title 2", "Clean 2", 1, 2.0, "", "", 0.9, "regex")
	if err := ClearParserCache(); err != nil {
		t.Fatalf("ClearParserCache 失败: %v", err)
	}
	if _, ok := GetCachedTitleResult("Title 1"); ok {
		t.Errorf("清空后 Title 1 不应命中")
	}
}

// TestParserCache_NilDBSafetyAndBounds 测试 database.DB 为 nil 时的安全性与超长截断保护
func TestParserCache_NilDBSafetyAndBounds(t *testing.T) {
	// 备份原 DB 引用
	originalDB := database.DB
	database.DB = nil
	defer func() {
		database.DB = originalDB
	}()

	// nil DB 下读写不应发生 panic
	if _, ok := GetCachedTitleResult("Any Title"); ok {
		t.Errorf("DB 为 nil 时期望返回 false")
	}
	if err := SaveCachedTitleResult("Any Title", "Clean", 1, 1.0, "", "", 0.9, "regex"); err != nil {
		t.Errorf("DB 为 nil 时 Save 不应报错")
	}
	if err := DeleteCachedTitleResult("Any Title"); err != nil {
		t.Errorf("DB 为 nil 时 Delete 不应报错")
	}
	if err := ClearParserCache(); err != nil {
		t.Errorf("DB 为 nil 时 Clear 不应报错")
	}

	// 恢复 DB 并测试超长字符串
	database.DB = originalDB
	superLongRaw := "\x00\u200B" + strings.Repeat("超长测试标题字符", 500)
	norm := NormalizeCacheTitle(superLongRaw)
	if len([]rune(norm)) > maxCacheTitleRunes {
		t.Errorf("NormalizeCacheTitle 未按上限截断字符: %d", len([]rune(norm)))
	}
	if strings.Contains(norm, "\x00") || strings.Contains(norm, "\u200B") {
		t.Errorf("NormalizeCacheTitle 未清除控制字符与零宽空格")
	}
}
