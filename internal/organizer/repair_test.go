package organizer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func repairFixture(t *testing.T) (LibraryVolume, database.Episode, string) {
	t.Helper()
	dir := t.TempDir()
	lib := filepath.Join(dir, "library")
	srcDir := filepath.Join(dir, "downloads")
	for _, p := range []string{filepath.Join(lib, "Season 1"), srcDir} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	src := filepath.Join(srcDir, "edition.mkv")
	writeVersion(t, src, "selected edition")
	before, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	writeVersion(t, filepath.Join(lib, volumeMarker), "library-a")
	return LibraryVolume{Root: lib, ID: "library-a"}, database.Episode{Status: "organized", DownloadRoot: srcDir, SourceFile: filepath.Base(src), SourceModTime: before.ModTime().UnixNano(), FileSize: before.Size(), FinalPath: filepath.Join(lib, "Season 1", "episode [v-e2].mkv")}, src
}
func TestRestoreLibraryLink(t *testing.T) {
	v, ep, src := repairFixture(t)
	root := repairStat(t, v.Root)
	type restoreLink interface {
		RestoreLibraryLink(context.Context, database.Episode, string, os.FileInfo) error
	}
	org, ok := any(v).(restoreLink)
	if !ok {
		t.Fatal("missing hard-link repair for recorded library path")
	}
	for i := 0; i < 2; i++ {
		if err := org.RestoreLibraryLink(t.Context(), ep, src, root); err != nil {
			t.Fatal(err)
		}
	}
	a, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(ep.FinalPath)
	if err != nil || !os.SameFile(a, b) {
		t.Fatalf("not the same file: %v", err)
	}
	if err := os.Remove(ep.FinalPath); err != nil {
		t.Fatal(err)
	}
	writeVersion(t, ep.FinalPath, "selected edition")
	if err := org.RestoreLibraryLink(t.Context(), ep, src, root); err == nil {
		t.Fatal("accepted a different file with identical contents")
	}
	b, err = os.Stat(ep.FinalPath)
	if err != nil || os.SameFile(a, b) {
		t.Fatal("overwrote existing file", err)
	}
	if err := os.Remove(ep.FinalPath); err != nil {
		t.Fatal(err)
	}
	writeVersion(t, filepath.Join(v.Root, volumeMarker), "wrong")
	if err := org.RestoreLibraryLink(t.Context(), ep, src, root); err == nil {
		t.Fatal("wrote to wrong volume")
	}
	if _, err := os.Stat(ep.FinalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unexpected target", err)
	}
}

func repairStat(t *testing.T, name string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestRestoreLibraryLinkRequiresEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*database.Episode)
	}{
		{"different size", func(ep *database.Episode) { ep.FileSize++ }},
		{"different modification time", func(ep *database.Episode) { ep.SourceModTime++ }},
		{"unknown modification time", func(ep *database.Episode) { ep.SourceModTime = 0 }},
		{"unbound source", func(ep *database.Episode) { ep.SourceFile = "" }},
		{"noncanonical source", func(ep *database.Episode) { ep.SourceFile = "subdir/../" + ep.SourceFile }},
		{"different status", func(ep *database.Episode) { ep.Status = "downloaded" }},
		{"non-media target", func(ep *database.Episode) { ep.FinalPath += ".txt" }},
		{"outside library", func(ep *database.Episode) { ep.FinalPath = filepath.Join(ep.DownloadRoot, "other.mkv") }},
		{"missing parent", func(ep *database.Episode) {
			ep.FinalPath = filepath.Join(filepath.Dir(ep.FinalPath), "missing", "ep.mkv")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, ep, src := repairFixture(t)
			tc.change(&ep)
			if err := v.RestoreLibraryLink(t.Context(), ep, src, repairStat(t, v.Root)); err == nil {
				t.Fatal("accepted invalid repair")
			}
			if _, err := os.Lstat(ep.FinalPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("target appeared: %v", err)
			}
			if tc.name == "missing parent" {
				if _, err := os.Lstat(filepath.Dir(ep.FinalPath)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("parent appeared: %v", err)
				}
			}
			if b, err := os.ReadFile(src); err != nil || string(b) != "selected edition" {
				t.Fatalf("source changed: %q %v", b, err)
			}
		})
	}
}

func TestRestoreLibraryLinkUnmarkedRootIdentity(t *testing.T) {
	v, ep, src := repairFixture(t)
	v.ID = ""
	if err := os.Remove(filepath.Join(v.Root, volumeMarker)); err != nil {
		t.Fatal(err)
	}
	for _, root := range []os.FileInfo{nil, repairStat(t, ep.DownloadRoot)} {
		if err := v.RestoreLibraryLink(t.Context(), ep, src, root); err == nil {
			t.Fatal("accepted different or unconfirmed root")
		}
		if _, err := os.Lstat(ep.FinalPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	if err := v.RestoreLibraryLink(t.Context(), ep, src, repairStat(t, v.Root)); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreLibraryLinkRefusesSymlink(t *testing.T) {
	for _, kind := range []string{"source", "target", "parent"} {
		t.Run(kind, func(t *testing.T) {
			v, ep, src := repairFixture(t)
			other := filepath.Join(v.Root, "other")
			if err := os.Mkdir(other, 0700); err != nil {
				t.Fatal(err)
			}
			var link, destination string
			switch kind {
			case "source":
				link, destination = src, filepath.Join(ep.DownloadRoot, "renamed.mkv")
				if err := os.Rename(link, destination); err != nil {
					t.Fatal(err)
				}
			case "target":
				link, destination = ep.FinalPath, src
			case "parent":
				link, destination = filepath.Dir(ep.FinalPath), other
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(destination, link); err != nil {
				t.Skipf("creating symlinks requires OS privileges: %v", err)
			}
			if err := v.RestoreLibraryLink(t.Context(), ep, src, repairStat(t, v.Root)); err == nil {
				t.Fatal("accepted symlink")
			}
			if entries, err := os.ReadDir(other); err != nil || len(entries) != 0 {
				t.Fatalf("symlink target changed: %v %v", entries, err)
			}
			if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("replaced link: %v %v", info, err)
			}
		})
	}
}

type repairCheckContext struct {
	context.Context
	checks  int
	onCheck func(int)
}

func (c *repairCheckContext) Err() error {
	c.checks++
	c.onCheck(c.checks)
	return c.Context.Err()
}

func TestRestoreLibraryLinkPathReplacement(t *testing.T) {
	for _, kind := range []string{"source", "parent", "final entry"} {
		t.Run(kind, func(t *testing.T) {
			v, ep, src := repairFixture(t)
			root := repairStat(t, v.Root)
			redirected := filepath.Join(filepath.Dir(ep.FinalPath)+".original", filepath.Base(ep.FinalPath))
			changed := false
			ctx := &repairCheckContext{Context: t.Context(), onCheck: func(n int) {
				// 第三次检查在排他发布前，第四次在最终核验时。
				if kind == "final entry" && n != 4 || kind != "final entry" && n != 3 {
					return
				}
				switch kind {
				case "source":
					if err := os.Rename(src, src+".original"); err != nil {
						if runtime.GOOS == "windows" {
							return
						}
						t.Fatal(err)
					}
					writeVersion(t, src, "foreign content!")
				case "parent":
					parent := filepath.Dir(ep.FinalPath)
					if err := os.Rename(parent, parent+".original"); err != nil {
						if runtime.GOOS == "windows" {
							return
						}
						t.Fatal(err)
					}
					if err := os.Mkdir(parent, 0700); err != nil {
						t.Fatal(err)
					}
				case "final entry":
					if err := os.Remove(ep.FinalPath); err != nil {
						if runtime.GOOS == "windows" {
							return
						}
						t.Fatal(err)
					}
					writeVersion(t, ep.FinalPath, "foreign content!")
				}
				changed = true
			}}
			err := v.RestoreLibraryLink(ctx, ep, src, root)
			if !changed && runtime.GOOS == "windows" && err == nil {
				if !os.SameFile(repairStat(t, src), repairStat(t, ep.FinalPath)) {
					t.Fatal("opened handles did not preserve source")
				}
				return
			}
			if !changed || err == nil {
				t.Fatalf("path change accepted: changed=%v err=%v", changed, err)
			}
			if kind == "final entry" {
				if b, err := os.ReadFile(ep.FinalPath); err != nil || string(b) != "foreign content!" {
					t.Fatalf("foreign final entry changed: %q %v", b, err)
				}
			} else {
				for _, p := range []string{ep.FinalPath, redirected} {
					if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("unexpected target %s: %v", p, err)
					}
				}
			}
		})
	}
}

func TestRestoreLibraryLinkConcurrent(t *testing.T) {
	v, ep, source := repairFixture(t)
	root := repairStat(t, v.Root)
	ctx := t.Context()
	start := make(chan struct{})
	result := make(chan error, 8)
	for i := 0; i < cap(result); i++ {
		go func() { <-start; result <- v.RestoreLibraryLink(ctx, ep, source, root) }()
	}
	close(start)
	for i := 0; i < cap(result); i++ {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
	if !os.SameFile(repairStat(t, source), repairStat(t, ep.FinalPath)) {
		t.Fatal("target identity changed")
	}
	entries, err := os.ReadDir(filepath.Dir(ep.FinalPath))
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
}

func TestRestoreLibraryLinkCrossVolume(t *testing.T) {
	v, ep, src := repairFixture(t)
	dir, err := os.MkdirTemp("/dev/shm", "anigo-repair-")
	if err != nil {
		t.Skip("second writable filesystem unavailable")
	}
	defer os.RemoveAll(dir)
	if err := os.Link(src, filepath.Join(dir, "probe")); err == nil {
		t.Skip("fixture uses a single filesystem")
	}
	ep.DownloadRoot = dir
	source := filepath.Join(dir, ep.SourceFile)
	writeVersion(t, source, "selected edition")
	ep.SourceModTime = repairStat(t, source).ModTime().UnixNano()
	err = v.RestoreLibraryLink(t.Context(), ep, source, repairStat(t, v.Root))
	if !errors.Is(err, ErrLinkUnavailable) {
		t.Fatalf("cross-volume result: %v", err)
	}
	if _, err := os.Lstat(ep.FinalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("produced a target: %v", err)
	}
	if b, err := os.ReadFile(source); err != nil || string(b) != "selected edition" {
		t.Fatalf("source changed: %q %v", b, err)
	}
}
