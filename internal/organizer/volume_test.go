package organizer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestOrganizeRequiresMatchingLibraryVolume(t *testing.T) {
	for _, tc := range []struct{ kind, key string }{{"tv", "TV_VOLUME_ID"}, {"Movie", "MOVIE_VOLUME_ID"}, {"OVA", "OVA_VOLUME_ID"}} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Setenv(tc.key, "volume-A")
			p := t.TempDir()
			root := filepath.Join(p, "library")
			src := filepath.Join(p, "original.mkv")
			if err := os.WriteFile(src, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			o := New("Show/E{ep:02}.mkv", "Show/movie.mkv", "Show/special.mkv", root, root, false, nil, root)
			anime, ep := core.Anime{Type: tc.kind}, core.Episode{Season: 1, Number: 1}
			commitCount := 0
			commit := func(string, *MoveSource) error { commitCount++; return nil }
			if _, err := o.OrganizeVersionAndCommit(t.Context(), src, anime, ep, "v1", commit); err == nil || commitCount != 0 {
				t.Fatalf("absent protected root was published, commits=%d error=%v", commitCount, err)
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatalf("created absent root: %v", err)
			}
			if err := os.Mkdir(root, 0755); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(root, ".anigo-volume-id")
			for _, wrong := range []string{"", "other volume\n", "volume-A\njunk"} {
				if err := os.WriteFile(marker, []byte(wrong), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := o.Organize(context.Background(), src, anime, ep); err == nil {
					t.Fatal("manual move accepted wrong marker")
				}
				if _, err := os.Stat(src); err != nil {
					t.Fatal("lost source", err)
				}
				if _, err := os.Stat(filepath.Join(root, "Show")); !os.IsNotExist(err) {
					t.Fatal("created target on wrong volume", err)
				}
			}
			if err := os.WriteFile(marker, []byte("volume-A\r\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := o.OrganizeVersionAndCommit(t.Context(), src, anime, ep, "v1", commit); err != nil || commitCount != 1 {
				t.Fatalf("returning volume: commits=%d error=%v", commitCount, err)
			}
		})
	}
}

type replaceVolumeDuringStage struct {
	context.Context
	library string
	changed bool
}

func (c *replaceVolumeDuringStage) Err() error {
	if !c.changed {
		staged, err := filepath.Glob(filepath.Join(c.library, "Series", stagePrefix+"*", "media"))
		if err != nil {
			return err
		}
		if len(staged) != 0 {
			if err := os.WriteFile(filepath.Join(c.library, volumeMarker), []byte("other"), 0600); err != nil {
				return err
			}
			c.changed = true
		}
	}
	return c.Context.Err()
}

func TestVolumeChangesBeforePublishingStage(t *testing.T) {
	t.Setenv("TV_VOLUME_ID", "expected")
	for _, versioned := range []bool{false, true} {
		lib := t.TempDir()
		src := filepath.Join(t.TempDir(), "source.mkv")
		writeVersion(t, filepath.Join(lib, volumeMarker), "expected")
		writeVersion(t, src, "complete")
		o := versionFixture(lib)
		ctx := &replaceVolumeDuringStage{Context: t.Context(), library: lib}
		commit := func(string, *MoveSource) error { t.Fatal("committed after volume change"); return nil }
		anime, ep := core.Anime{Type: "tv"}, core.Episode{Season: 1, Number: 1}
		var err error
		if versioned {
			_, err = o.OrganizeVersionAndCommit(ctx, src, anime, ep, "v1", commit)
		} else {
			_, err = o.OrganizeAndCommit(ctx, src, anime, ep, commit)
		}
		if err == nil || !ctx.changed {
			t.Fatalf("volume changed=%v, organize error=%v", ctx.changed, err)
		}
		if _, err := os.Lstat(filepath.Join(lib, "Series", "S01E01.mkv")); !os.IsNotExist(err) {
			t.Fatal("published on wrong volume", err)
		}
		if data, err := os.ReadFile(src); err != nil || string(data) != "complete" {
			t.Fatal("lost source after marker changed", err)
		}
	}
}

func TestGuardedLibraryRejectsLinkedTargetDirectory(t *testing.T) {
	lib, other := t.TempDir(), t.TempDir()
	t.Setenv("TV_VOLUME_ID", "expected")
	writeVersion(t, filepath.Join(lib, volumeMarker), "expected")
	if err := os.Symlink(other, filepath.Join(lib, "Series")); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}
	src := filepath.Join(t.TempDir(), "source.mkv")
	writeVersion(t, src, "video")
	o := versionFixture(lib)
	if _, err := o.Organize(t.Context(), src, core.Anime{Type: "tv"}, core.Episode{}); err == nil {
		t.Fatal("published outside marked root")
	}
	entries, err := os.ReadDir(other)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside root modified: %v %v", entries, err)
	}
}
