package organizer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLibrarySourceRoot(t *testing.T) {
	base := t.TempDir()
	tv, movie := filepath.Join(base, "tv"), filepath.Join(base, "movies")
	season := filepath.Join(tv, "series", "season")
	for _, dir := range []string{season, movie, filepath.Join(base, "tv-old")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	episode := filepath.Join(season, "E00.mkv")
	outside := filepath.Join(base, "tv-old", "E00.mkv")
	empty := filepath.Join(season, "empty.mkv")
	for name, contents := range map[string]string{episode: "episode", outside: "outside", empty: ""} {
		if err := os.WriteFile(name, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	o := New("", "", "", tv, movie, true, nil)
	check := func(kind, path, want string) {
		t.Helper()
		got, ok := o.LibrarySourceRoot(kind, path)
		if ok != (want != "") || got != want {
			t.Errorf("%s %q: 匹配到 %q, %v，期望根目录 %q", kind, path, got, ok, want)
		}
	}
	check("tv", episode, tv)
	for _, path := range []string{outside, empty, tv, season, "", filepath.Join(season, "missing"), season + string(filepath.Separator) + "../season/E00.mkv"} {
		check("tv", path, "")
	}
	check("movie", episode, "")

	t.Run("符号链接", func(t *testing.T) {
		within := filepath.Join(tv, "another-season")
		if err := os.Symlink(season, within); err != nil {
			t.Skipf("无法创建测试符号链接: %v", err)
		}
		for link, target := range map[string]string{filepath.Join(tv, "outside"): filepath.Dir(outside), filepath.Join(tv, "linked.mkv"): episode} {
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
		}
		check("tv", filepath.Join(within, "E00.mkv"), "")
		check("tv", filepath.Join(tv, "outside", "E00.mkv"), "")
		check("tv", filepath.Join(tv, "linked.mkv"), "")
	})
}
