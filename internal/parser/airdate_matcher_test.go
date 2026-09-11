package parser

import (
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestMatchEpisodeByAirDate(t *testing.T) {
	// 模拟 Bangumi 第二季剧集列表 (第二季共 12 集，单集从 1 开始)
	// 第 1 集于 2024-04-06 播出，第 2 集于 2024-04-13 播出
	airDateEp1, _ := time.Parse("2006-01-02", "2024-04-06")
	airDateEp2, _ := time.Parse("2006-01-02", "2024-04-13")

	eps := []core.Episode{
		{
			AnimeID: "12345",
			Season:  2,
			Number:  1,
			Title:   "新篇开幕",
			AiredAt: airDateEp1,
		},
		{
			AnimeID: "12345",
			Season:  2,
			Number:  2,
			Title:   "重逢之时",
			AiredAt: airDateEp2,
		},
	}

	// 测试用例 1：种子在第 1 集播出后 10 小时发布，标题带有数字 1
	pubTime1 := airDateEp1.Add(10 * time.Hour)
	res1 := MatchEpisodeByAirDate(pubTime1, "【字幕组】[某番剧 S2][01][1080P]", eps)
	if !res1.Matched || res1.Season != 2 || res1.Episode != 1 {
		t.Fatalf("用例 1 匹配失败: %+v", res1)
	}
	if res1.Confidence < 0.9 {
		t.Errorf("期望置信度 >= 0.9，实际为: %f", res1.Confidence)
	}

	// 测试用例 2：种子在第 2 集播出后 15 小时发布，标题没有明确集数前缀但带有 2
	pubTime2 := airDateEp2.Add(15 * time.Hour)
	res2 := MatchEpisodeByAirDate(pubTime2, "【字幕组】某番剧 2 简日双语", eps)
	if !res2.Matched || res2.Season != 2 || res2.Episode != 2 {
		t.Fatalf("用例 2 匹配失败: %+v", res2)
	}

	// 测试用例 3：种子发布时间超出所有剧集的 72 小时窗口
	oldPubTime := airDateEp1.Add(-100 * time.Hour)
	res3 := MatchEpisodeByAirDate(oldPubTime, "【字幕组】[某番剧][01]", eps)
	if res3.Matched {
		t.Fatalf("用例 3 不应匹配，但返回了: %+v", res3)
	}
}
