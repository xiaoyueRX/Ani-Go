package parser

import (
	"context"
	"errors"
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

// TestParseAirDate_Timezones 测试不同时区与格式的日期时间解析
func TestParseAirDate_Timezones(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantHour  int
		wantValid bool
	}{
		{
			name:      "RFC3339 UTC",
			input:     "2024-04-06T15:30:00Z",
			wantHour:  15,
			wantValid: true,
		},
		{
			name:      "RFC3339 东八区",
			input:     "2024-04-06T23:30:00+08:00",
			wantHour:  15, // 转换为 UTC 后为 15:30
			wantValid: true,
		},
		{
			name:      "带 CST 字样的格式",
			input:     "2024-04-06 23:00:00 CST",
			wantHour:  15, // 23:00 - 8h = 15:00 UTC
			wantValid: true,
		},
		{
			name:      "纯年月日日期",
			input:     "2024-04-06",
			wantHour:  0,
			wantValid: true,
		},
		{
			name:      "空白字符串",
			input:     "   ",
			wantValid: false,
		},
		{
			name:      "无法解析的非法格式",
			input:     "invalid-date-format",
			wantValid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseAirDate(tc.input)
			if tc.wantValid {
				if err != nil {
					t.Fatalf("ParseAirDate(%q) 期望成功，实际失败: %v", tc.input, err)
				}
				if parsed.Hour() != tc.wantHour {
					t.Errorf("UTC 小时数期望为 %d, 实际为 %d", tc.wantHour, parsed.Hour())
				}
			} else {
				if err == nil {
					t.Fatalf("ParseAirDate(%q) 期望报错，实际返回了: %v", tc.input, parsed)
				}
			}
		})
	}
}

// TestMatchEpisodeByAirDate_ToleranceWindow 测试 -6h 到 +72h 容差窗口边界
func TestMatchEpisodeByAirDate_ToleranceWindow(t *testing.T) {
	airDate, _ := time.Parse("2006-01-02", "2024-10-01")
	eps := []core.Episode{
		{
			Season:  1,
			Number:  1,
			AiredAt: airDate,
		},
	}

	// 提前 5 小时（在 -6h 窗口内）：应匹配
	resEarly := MatchEpisodeByAirDate(airDate.Add(-5*time.Hour), "【字幕组】先行上映第1集", eps)
	if !resEarly.Matched {
		t.Errorf("先行上映 (-5h) 应匹配成功")
	}

	// 提前 7 小时（超出 -6h 窗口）：不应匹配
	resTooEarly := MatchEpisodeByAirDate(airDate.Add(-7*time.Hour), "【字幕组】提前7小时", eps)
	if resTooEarly.Matched {
		t.Errorf("提前 7 小时超出容差，不应匹配")
	}

	// 播出后 71 小时（在 72h 窗口内）：应匹配
	resLate := MatchEpisodeByAirDate(airDate.Add(71*time.Hour), "【字幕组】压制版第1集", eps)
	if !resLate.Matched {
		t.Errorf("播出后 71h 应匹配成功")
	}

	// 播出后 75 小时（超出 72h 窗口）：不应匹配
	resTooLate := MatchEpisodeByAirDate(airDate.Add(75*time.Hour), "【字幕组】超期发布第1集", eps)
	if resTooLate.Matched {
		t.Errorf("播出后 75h 超出容差，不应匹配")
	}

	// 种子发布时间为 Zero
	resZero := MatchEpisodeByAirDate(time.Time{}, "种子标题", eps)
	if resZero.Matched {
		t.Errorf("发布时间为零值时不应匹配")
	}
}

// TestMatchEpisodeByAirDate_TieBreaking 测试无数字匹配时的最佳 12 小时延迟优先决胜
func TestMatchEpisodeByAirDate_TieBreaking(t *testing.T) {
	baseTime, _ := time.Parse("2006-01-02 15:04:05", "2024-10-01 00:00:00")
	// 两个剧集同时在容差范围内：
	// epA: 播出后 12 小时 (距离理想 12h 延迟差 0h)
	// epB: 播出后 30 小时 (距离理想 12h 延迟差 18h)
	epA := core.Episode{Season: 1, Number: 1, AiredAt: baseTime}
	epB := core.Episode{Season: 1, Number: 2, AiredAt: baseTime.Add(-18 * time.Hour)}

	pubTime := baseTime.Add(12 * time.Hour)
	eps := []core.Episode{epA, epB}

	// 标题不包含数字 1 或 2，测试基于最佳时间延迟的仲裁
	res := MatchEpisodeByAirDate(pubTime, "【字幕组】未标集数纯标题 1080p", eps)
	if !res.Matched || res.Episode != 1 {
		t.Fatalf("决胜期望匹配 epA (Ep 1), 实际匹配: %+v", res)
	}
}

type mockMetadataProvider struct {
	episodes []core.Episode
	err      error
}

func (m *mockMetadataProvider) Name() string { return "mock" }
func (m *mockMetadataProvider) SearchAnime(_ context.Context, _ string) ([]core.Anime, error) {
	return nil, nil
}
func (m *mockMetadataProvider) GetAnime(_ context.Context, _ string) (core.Anime, error) {
	return core.Anime{}, nil
}
func (m *mockMetadataProvider) GetEpisodes(_ context.Context, _ string, _ int) ([]core.Episode, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.episodes, nil
}
func (m *mockMetadataProvider) GetCollections(_ context.Context, _ string) ([]core.Anime, error) {
	return nil, nil
}

// TestMatchEpisodeFromProvider 测试从元数据提供方调用分集对齐的异常安全保护
func TestMatchEpisodeFromProvider(t *testing.T) {
	airDate, _ := time.Parse("2006-01-02", "2024-04-06")
	provider := &mockMetadataProvider{
		episodes: []core.Episode{
			{Season: 1, Number: 1, AiredAt: airDate},
		},
	}

	// 1. 正常对齐
	res, err := MatchEpisodeFromProvider(context.Background(), provider, "12345", 1, airDate.Add(8*time.Hour), "[Subs] Title [01] 1080p")
	if err != nil || !res.Matched || res.Episode != 1 {
		t.Fatalf("正常对齐失败: res=%+v, err=%v", res, err)
	}

	// 2. Provider 为 nil 保护
	resNil, errNil := MatchEpisodeFromProvider(context.Background(), nil, "12345", 1, airDate.Add(8*time.Hour), "[Subs] Title [01]")
	if errNil == nil || resNil.Matched {
		t.Errorf("Provider 为 nil 时应报错且返回 Matched=false")
	}

	// 3. 上下文取消保护
	ctxCancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, errCancel := MatchEpisodeFromProvider(ctxCancelled, provider, "12345", 1, airDate.Add(8*time.Hour), "[Subs] Title [01]")
	if errCancel == nil {
		t.Errorf("上下文取消应报错")
	}

	// 4. Provider 接口报错保护
	errProvider := &mockMetadataProvider{err: errors.New("network error")}
	_, errNet := MatchEpisodeFromProvider(context.Background(), errProvider, "12345", 1, airDate.Add(8*time.Hour), "[Subs] Title [01]")
	if errNet == nil {
		t.Errorf("Provider 报错时应透传错误")
	}
}
