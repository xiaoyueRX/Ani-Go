package organizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestMoveBetweenEquivalentFileNamesPreservesMedia(t *testing.T) {
	for _, tc := range []struct{ name, source, title string }{
		{"case", "episode.mkv", "Episode"},
		{"Unicode composition", "Cafe\u0301.mkv", "Café"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, tc.source)
			dst := filepath.Join(dir, tc.title+".mkv")
			const content = "complete episode"
			if err := os.WriteFile(src, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := os.Stat(dst)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			alias := err == nil
			t.Logf("target spelling already resolves to the source: %v", alias)
			org := New("{title_cn}{ext}", "", "", dir, "", false, nil)
			path, err := org.Organize(t.Context(), src, core.Anime{Type: "TV", TitleCN: tc.title}, core.Episode{Season: 1, Number: 1})
			if err != nil || path != dst {
				t.Fatalf("renaming %q: got %q, err = %v", src, path, err)
			}
			if got, err := os.ReadFile(dst); err != nil || string(got) != content {
				t.Fatalf("media missing or changed at %q: %q, %v", dst, got, err)
			}
			if !alias {
				if _, err := os.Stat(src); !os.IsNotExist(err) {
					t.Fatalf("old name still exists after move: %v", err)
				}
			}
		})
	}
}
