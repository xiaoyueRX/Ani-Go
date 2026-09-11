package source

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
)

func TestParseMikanTitleAdvanced_AirdateAlignment(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_advanced.db")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("db init failed: %v", err)
	}

	// 模拟剧集：S02E01 对应总集数 25
	airDate := time.Date(2024, 7, 5, 0, 0, 0, 0, time.UTC)
	pubDate := airDate.Add(8 * time.Hour) // 播出后 8 小时发布种子

	episodes := []core.Episode{
		{
			Season:  2,
			Number:  1,
			AiredAt: airDate,
		},
	}

	// 疑难标题：只包含 [25]（没有标注 S2E01）
	rawTitle := "[Sakurato] Jujutsu Kaisen - 25 [1080p HEVC]"

	// 第一次解析：未缓存，调用时间戳对齐
	info := ParseMikanTitleAdvanced(rawTitle, pubDate, episodes)
	if info.Season != 2 || info.Episode != 1 {
		t.Errorf("expected S02E01 after airdate matching, got S%02dE%02d", info.Season, int(info.Episode))
	}

	// 验证已自动写入持久化缓存
	cached, ok := parser.GetCachedTitleResult(rawTitle)
	if !ok {
		t.Fatalf("expected title to be cached, but was not")
	}
	if cached.Season != 2 || cached.Episode != 1 {
		t.Errorf("cached result mismatch: %+v", cached)
	}
	if cached.ResolvedBy != "airdate" {
		t.Errorf("expected resolved_by 'airdate', got '%s'", cached.ResolvedBy)
	}

	// 第二次解析：直接命中缓存
	cachedInfo := ParseMikanTitleAdvanced(rawTitle, time.Time{}, nil)
	if cachedInfo.Season != 2 || cachedInfo.Episode != 1 {
		t.Errorf("expected S02E01 from cache, got S%02dE%02d", cachedInfo.Season, int(cachedInfo.Episode))
	}
}
