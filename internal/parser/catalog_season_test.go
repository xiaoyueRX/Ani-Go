package parser

import "testing"

func TestCatalogSeasonUnambiguousMarkers(t *testing.T) {
	for _, tc := range []struct {
		title string
		want  int
	}{
		{"星岛 第2季", 2}, {"星岛 第二季", 2}, {"星岛 第二十二季", 22},
		{"Harbor 2nd Season", 2}, {"Harbor Season 3", 3}, {"星岛第二季 / Harbor S02", 2},
		{"星岛 S02E03", 2}, {"星岛 2026", 1}, {"星岛 第29话", 1}, {"星岛 第二三季", 1},
		{"星岛 第一至第三季", 1}, {"星岛 第2季和第3季", 1}, {"星岛 第二季 / Season 3", 1},
		{"Harbor S02/S03", 1}, {"Harbor S2-4", 1}, {"Harbor Season 2 and 3", 1},
		{"Harbor Special S00", 1}, {"Harbor S02E03-S03E02", 1},
	} {
		t.Run(tc.title, func(t *testing.T) {
			if got := CatalogSeason(tc.title); got != tc.want {
				t.Fatalf("季号=%d，期望%d", got, tc.want)
			}
		})
	}
}
