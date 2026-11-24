package parser

import "testing"

func TestDirectorySeason(t *testing.T) {
	for _, tc := range []struct {
		rel       string
		season    int
		explicit  bool
		ambiguous bool
	}{
		{"01.mkv", 0, false, false},
		{"Pack/01.mkv", 0, false, false},
		{"Pack (2026)/CD1/01.mkv", 0, false, false},
		{"Season 1/01.mkv", 1, true, false},
		{"Season 2/01.mkv", 2, true, false},
		{"第二季/01.mkv", 2, true, false},
		{"Show S03/Extras/01.mkv", 3, true, false},
		{"2nd Season/01.mkv", 2, true, false},
		{"Season 0/01.mkv", 0, true, false},
		{"S00/Show S00E01.mkv", 0, true, false},
		{"第0季/01.mkv", 0, true, false},
		{"Season 2/Season 02/01.mkv", 2, true, false},
		{"Season 1-2/01.mkv", 0, false, true},
		{"S01-S02/01.mkv", 0, false, true},
		{"Season 1/Season 2/01.mkv", 0, false, true},
		{"第二至第三季/01.mkv", 0, false, true},
		{"Season 2/Extras/S00/01.mkv", 0, false, true},
	} {
		t.Run(tc.rel, func(t *testing.T) {
			gotSeason, gotExplicit, gotAmbiguous := DirectorySeason(tc.rel)
			if gotSeason != tc.season || gotExplicit != tc.explicit || gotAmbiguous != tc.ambiguous {
				t.Fatalf("DirectorySeason(%q) = (%d, %v, %v), want (%d, %v, %v)",
					tc.rel, gotSeason, gotExplicit, gotAmbiguous, tc.season, tc.explicit, tc.ambiguous)
			}
		})
	}
}
