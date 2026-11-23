package core

import "testing"

func TestMovieExtraIgnoresTitleButKeepsExplicitReleaseMarkers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		titles []string
		extra  bool
	}{
		{"The.Menu.2022.mkv", []string{"The", "The Menu"}, false},
		{"The Menu/The.Menu.mkv", []string{"The Menu"}, false},
		{"[Group] Trailer Park Boys 1080p.mkv", []string{"Trailer Park Boys"}, false},
		{"Trailer Park Boys - Trailer 2.mkv", []string{"Trailer Park Boys"}, true},
		{"[Film] NCOP01 [1080p].mkv", []string{"Film"}, true},
		{"Film/Featurettes/Interview 1.mkv", []string{"Film"}, true},
		{"花絮之夏 [1080p].mkv", []string{"花絮之夏"}, false},
		{"Film - Bonus Short.mkv", []string{"Film"}, true},
		{"[短片] Film.mkv", []string{"Film"}, false},
		{"The Big Short.mkv", []string{"The Big Short"}, false},
		{"花絮之夏 [劇場予告].mkv", []string{"花絮之夏"}, true},
		{"[Previewing Films] Feature [OPUS].mkv", []string{"Feature"}, false},
	} {
		if got := MovieExtra(tc.name, tc.titles...); got != tc.extra {
			t.Errorf("%q：附件=%v，期望 %v", tc.name, got, tc.extra)
		}
	}
}
