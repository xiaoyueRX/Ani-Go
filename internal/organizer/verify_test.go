package organizer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func TestVerifyHistoricalLibraryPaths(t *testing.T) {
	t.Setenv("TV_VOLUME_ID", "")
	t.Chdir(t.TempDir())
	configured := filepath.Join("local", "shows")
	if err := os.MkdirAll(configured, 0700); err != nil {
		t.Fatal(err)
	}
	base, err := filepath.Abs(configured)
	if err != nil {
		t.Fatal(err)
	}
	o := New("", "", "", configured, t.TempDir(), true, nil)
	check := func(path, status, reason string) {
		t.Helper()
		ep := database.Episode{Status: "organized", FinalPath: path, FileSize: 65536}
		results, _, err := o.VerifyLibraryFiles(context.Background(), "tv", []database.Episode{ep})
		if err != nil || len(results) != 1 || results[0].Status != status || results[0].Reason != reason {
			t.Fatalf("核对 %q: %+v %v，期望 %s/%s", path, results, err, status, reason)
		}
	}
	file := filepath.Join(configured, "Test S01E01.mkv")
	if err := os.WriteFile(file, []byte("media"), 0600); err != nil {
		t.Fatal(err)
	}
	check(file, "present", "")
	check(filepath.Join(base, "Test S01E01.mkv"), "present", "")
	check(filepath.Base(file), "unverified", "path")
	check(base+string(filepath.Separator)+".."+string(filepath.Separator)+filepath.Base(base)+string(filepath.Separator)+filepath.Base(file), "unverified", "path")
	check(`Z:\Films\Foreign S01E01.mkv`, "unverified", "path")
	check(`\\remote.invalid\lib\Foreign S01E01.mkv`, "unverified", "path")
	check(filepath.Join(base, "missing-dir", "Test S01E02.mkv"), "missing", "")
	dir := filepath.Join(base, "folder.mkv")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	check(dir, "unverified", "file_type")
	empty := filepath.Join(base, "empty.mkv")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	check(empty, "empty", "")
	if runtime.GOOS != "windows" {
		literal := filepath.Join(base, `member\literal.mkv`)
		if err := os.WriteFile(literal, []byte("media"), 0600); err != nil {
			t.Fatal(err)
		}
		check(literal, "present", "")
	}
	t.Run("拒绝链接文件和链接目录", func(t *testing.T) {
		outside := t.TempDir()
		linked := filepath.Join(base, "outside")
		if err := os.Symlink(outside, linked); err != nil {
			t.Skipf("不能建立符号链接：%v", err)
		}
		check(filepath.Join(linked, "missing.mkv"), "unverified", "file_type")
		leaf := filepath.Join(base, "pseudo.mkv")
		if err := os.Symlink(filepath.Join(outside, "missing.mkv"), leaf); err != nil {
			t.Fatal(err)
		}
		check(leaf, "unverified", "file_type")
	})
	t.Run("不能查目录时不能报告文件丢失", func(t *testing.T) {
		denied := filepath.Join(base, "private")
		if err := os.Mkdir(denied, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(denied, 0700) })
		file := filepath.Join(denied, "missing.mkv")
		if _, err := os.Stat(file); !os.IsPermission(err) {
			t.Skip("本机用户可以访问受限目录")
		}
		check(file, "unverified", "access")
	})
	if err := os.Rename(base, base+".offline"); err != nil {
		t.Fatal(err)
	}
	check(file, "unverified", "library_unavailable")
}
