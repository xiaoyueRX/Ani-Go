package scheduler

import (
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func TestBatchMemberEpisodeIdentity(t *testing.T) {
	sub := database.Subscription{TitleCN: "Series", Season: 2, TotalEpisodes: 10}
	for _, tc := range []struct {
		path   string
		season int
		ep     float32
		review bool
	}{
		{"Pack/Series S02E04.mkv", 2, 4, false},
		{"Pack/Extras/Series S00E15.mkv", 0, 15, false},
		{"Pack/Series SP02.mkv", 0, 2, false},
		{"Pack/Series - 04.5.mkv", 2, 4.5, false},
		{"Pack/Extras/Series Mini Anime - 04.mkv", 2, 4, true},
		{"Pack/Extra/Menu 1.mkv", 2, 1, true},
		{"Pack/NCOP02.mkv", 2, 2, true},
		{"Pack/Series - ED 01.mkv", 2, 1, true},
		{"Pack/[Group] Series S02E01 [OPUS].mkv", 2, 1, false},
		{"Pack/Ed and Friends S02E01.mkv", 2, 1, false},
		{"Pack/Series - NCOP01 [1080p].mkv", 2, 1, true},
		{"Pack/Series S02E29.mkv", 2, 29, true},
		{"Pack/Series S01E04.mkv", 1, 4, true},
		{"Season 2/01.mkv", 2, 1, false},
		{"Season 1/01.mkv", 2, 1, true},
		{"Season 1/01.mkv", 1, 1, true},
		{"Season 1-2/01.mkv", 2, 1, true},
		{"S01-S02/01.mkv", 2, 1, true},
		{"Season 1/Season 2/01.mkv", 2, 1, true},
		{"Season 0/01.mkv", 2, 1, true},
		{"Season 0/Series S00E01.mkv", 0, 1, false},
		{"Season 2/Extras/Series S00E15.mkv", 0, 15, false},
		{"Season 1/Extras/Series S00E15.mkv", 0, 15, true},
		{"Season 2/Series S01E01.mkv", 1, 1, true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			reason := batchEpisodeReviewReason(tc.path, sub, tc.season, tc.ep)
			if (reason != "") != tc.review {
				t.Errorf("文件 %s 对应 S%dE%g，复核原因 %q", tc.path, tc.season, tc.ep, reason)
			}
		})
	}
	if reason := batchEpisodeReviewReason("Season 2/01.mkv", database.Subscription{Season: 1, TotalEpisodes: 10}, 1, 1); reason == "" {
		t.Errorf("第一季订阅不应放行 Season 2 目录成员")
	}
	if reason := batchEpisodeReviewReason("E29.mkv", database.Subscription{Season: 2}, 2, 29); reason != "" {
		t.Errorf("未知总集数时不应将 E29 判为越界: %s", reason)
	}
}
