package parser

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// TestParser_ReDoSResistance 测试超长字符串与重复模式下的正则抗回溯拒绝服务（ReDoS）能力
func TestParser_ReDoSResistance(t *testing.T) {
	p := NewRegexParser()
	ctx := context.Background()

	testPatterns := []struct {
		name  string
		input string
	}{
		{
			name:  "50,000 Repeated Whitespaces and Brackets",
			input: strings.Repeat(" [第季 1080p] ", 5000),
		},
		{
			name:  "10,000 Repeated Chinese Digits",
			input: "追番 " + strings.Repeat("第十", 5000) + "季",
		},
		{
			name:  "30,000 Nested Parentheses",
			input: strings.Repeat("(((", 5000) + "Anime" + strings.Repeat(")))", 5000),
		},
		{
			name:  "Extremely Long Title with Spaces and Numbers",
			input: "订阅 " + strings.Repeat("A ", 15000) + "第2季 1080p",
		},
	}

	for _, tc := range testPatterns {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = p.Parse(ctx, tc.input)
				_, _ = ExtractEpisode(tc.input)
			}()

			select {
			case <-done:
				// 成功在合理时限内解析完成
			case <-time.After(2 * time.Second):
				t.Fatalf("[%s] 解析超时，疑似发生正则表达式灾难性回溯 (ReDoS)!", tc.name)
			}
		})
	}
}

// TestParser_ExtremeDirtyData 测试极端脏数据、非正常编码与冲突语法
func TestParser_ExtremeDirtyData(t *testing.T) {
	p := NewRegexParser()
	ctx := context.Background()

	dirtyInputs := []string{
		"",
		"   ",
		"\x00\x00\x00\r\n\t",
		"追番 🌈✨✨ 鬼灭之刃 ⚔️ 无限城篇 💥",
		"订阅 【喵萌奶茶屋】★01月新番★[我独自升级][01-12][720p][繁体][MP4]",
		"查 [DBD-Raws][SP01-04][1080P][BDRip][HEVC-10bit][FLAC]",
		"追番 间谍过家家 Season 2 S02 第2季 第二季 1080p 4K", // 冲突季数
		"订阅 咒术回战 第02.5话 [02.5] 1080p",
		"追番 葬送的芙莉莲 12v2",
		"退订",
		"追番 1080p 4K 720p", // 只有分辨率没有名字
		"\u202E\u200B\uFEFF逆向文字与零宽空格",
	}

	for i, input := range dirtyInputs {
		t.Run(fmt.Sprintf("Case_%d", i), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Input %q 引发了 panic: %v", input, r)
				}
			}()

			res, err := p.Parse(ctx, input)
			if err != nil {
				t.Logf("预期内的解析错误: %v", err)
			}
			s, ep := ExtractEpisode(input)
			_ = res
			_ = s
			_ = ep
		})
	}
}

// TestParser_HighConcurrencyStress 测试高并发任务解析与持久化解析缓存读写无数据竞争
func TestParser_HighConcurrencyStress(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "stress_cache.db")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init failed: %v", err)
	}

	p := NewRegexParser()

	const numWorkers = 30
	const numOps = 20

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	inputs := []string{
		"追番 进击的巨人 最终季 1080p",
		"订阅 鬼灭之刃 第3季",
		"追番 某科学的超电磁炮 第一季",
		"搜索 刀剑神域 S3",
		"退订 无职转生 第2季",
		"追番 【幻樱字幕组】葬送的芙莉莲 第05集 1080p",
	}

	for w := 0; w < numWorkers; w++ {
		workerID := w
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for i := 0; i < numOps; i++ {
				inp := inputs[(workerID+i)%len(inputs)]
				// 测试持久化缓存读写
				if _, ok := GetCachedTitleResult(inp); !ok {
					res, _ := p.Parse(ctx, inp)
					_ = SaveCachedTitleResult(inp, res.Title, res.Season, 1.0, "TestSub", res.Resolution, float32(res.Confidence), "regex")
				}
				// 测试种子集数提取
				_, _ = ExtractEpisode(inp)
			}
		}()
	}

	wg.Wait()
}
