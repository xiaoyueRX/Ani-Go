package parser

import "testing"

func TestDeclaredEpisodeRange(t *testing.T) {
	for _, tc := range []struct {
		title      string
		start, end int
	}{
		{"Series 2nd Season [01-10] [1080p]", 1, 10},
		{"Series 第29～38話 全10集", 29, 38},
		{"Series (EP01~10 END)【SP01-04】", 1, 10},
		{"Series [MAD] [01-10] 第1-10集", 1, 10},
		{"Series [01-10] [29-38]", 0, 0},
		{"Series (2023-2024) [1080p]", 0, 0},
		{"Series 全10集", 0, 0},
		{"Series [OVA01-02] [SP01-04] 合集", 0, 0},
		{"Series [10-01] 第0-2话", 0, 0},
	} {
		t.Run(tc.title, func(t *testing.T) {
			start, end := DeclaredEpisodeRange(tc.title)
			if start != tc.start || end != tc.end {
				t.Fatalf("解析到 %d–%d，预期 %d–%d", start, end, tc.start, tc.end)
			}
		})
	}
}
