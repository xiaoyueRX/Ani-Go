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

func TestMoveCleansOnlyTheOldHardlink(t *testing.T) {
	for _, aliasParent := range []bool{false, true} {
		name := "same directory"
		if aliasParent {
			name = "alias of same directory"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "library")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			publishedDir := dir
			if aliasParent {
				publishedDir = filepath.Join(root, "alias")
				if err := os.Symlink(dir, publishedDir); err != nil {
					t.Skipf("directory symlink unavailable: %v", err)
				}
			}
			src := filepath.Join(dir, "Café-release.mkv")
			const content = "complete episode with a seeding link"
			if err := os.WriteFile(src, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			seed := filepath.Join(dir, "seed.mkv")
			if err := os.Link(src, seed); err != nil {
				t.Skipf("hardlinks unavailable: %v", err)
			}
			dst := filepath.Join(publishedDir, "Episode.mkv")
			if err := os.Link(src, dst); err != nil {
				t.Fatal(err)
			}
			// 有些卷接受两种 Unicode 写法，下载器提供的路径可能与目录中保存的拼法不同。
			alternative := filepath.Join(dir, "Cafe\u0301-release.mkv")
			if _, err := os.Stat(alternative); err == nil {
				src = alternative
			}
			org := New("Episode{ext}", "", "", publishedDir, "", false, nil)
			path, err := org.Organize(t.Context(), src, core.Anime{Type: "TV"}, core.Episode{Season: 1, Number: 1})
			if err != nil || path != dst {
				t.Fatalf("move to existing hardlink: got %q, err = %v", path, err)
			}
			if _, err := os.Stat(src); !os.IsNotExist(err) {
				t.Fatalf("old entry still exists after move: %v", err)
			}
			for _, path := range []string{dst, seed} {
				if data, err := os.ReadFile(path); err != nil || string(data) != content {
					t.Fatalf("link %q lost or changed: %q, %v", path, data, err)
				}
			}
		})
	}
}
