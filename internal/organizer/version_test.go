package organizer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func versionFixture(lib string) *TVOrganizer {
	return New("Series/S01E01{ext}", "", "", lib, "", true, nil)
}

func writeVersion(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestVersionCollisionAndRetry(t *testing.T) {
	lib := t.TempDir()
	source := filepath.Join(t.TempDir(), "1080p.mkv")
	writeVersion(t, source, "高清版")
	original := filepath.Join(lib, "Series", "S01E01.mkv")
	if err := os.Mkdir(filepath.Dir(original), 0700); err != nil {
		t.Fatal(err)
	}
	writeVersion(t, original, "字幕版")
	o := versionFixture(lib)
	anime, episode := core.Anime{Type: "tv"}, core.Episode{Season: 1, Number: 1}
	preview, err := o.PreviewVersion(context.Background(), source, anime, episode, "torrent/path")
	if err != nil || preview.FinalPath == original || filepath.Ext(preview.FinalPath) != ".mkv" || preview.Exists {
		t.Fatalf("版本预览路径 %q，错误 %v", preview.FinalPath, err)
	}
	interrupted := errors.New("模拟数据库提交中断")
	_, err = o.OrganizeVersionAndCommit(context.Background(), source, anime, episode, "torrent/path", func(string, *MoveSource) error { return interrupted })
	if !errors.Is(err, interrupted) {
		t.Fatalf("提交中断应保留已发布版本: %v", err)
	}
	got, err := o.OrganizeVersionAndCommit(context.Background(), source, anime, episode, "torrent/path", func(string, *MoveSource) error { return nil })
	if err != nil || got != preview.FinalPath {
		t.Fatalf("重试复用原预览版本 %q，实际 %q (%v)", preview.FinalPath, got, err)
	}
	for path, want := range map[string]string{source: "高清版", got: "高清版", original: "字幕版"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("%q 内容 %q，期望 %q (%v)", path, data, want, err)
		}
	}
}

func TestConcurrentVersionPublication(t *testing.T) {
	const count = 8
	o := versionFixture(t.TempDir())
	dir := t.TempDir()
	start := make(chan struct{})
	type result struct {
		path, want string
		err        error
	}
	finished := make(chan result, count)
	for n := 0; n < count; n++ {
		contents := fmt.Sprintf("release-%d", n)
		src := filepath.Join(dir, contents+".mkv")
		writeVersion(t, src, contents)
		go func() {
			<-start
			dst, err := o.OrganizeVersionAndCommit(context.Background(), src, core.Anime{Type: "tv"}, core.Episode{Season: 1, Number: 1}, contents, func(string, *MoveSource) error { return nil })
			finished <- result{dst, contents, err}
		}()
	}
	close(start)
	records := make([]result, count)
	for i := range records {
		records[i] = <-finished
	}
	for _, r := range records {
		data, err := os.ReadFile(r.path)
		if r.err != nil || err != nil || string(data) != r.want {
			t.Fatalf("并发发布 %s 时保留了 %q，读取和整理错误 %v, %v", r.want, data, err, r.err)
		}
	}
}

func TestLongVersionNameFitsExistingMediaDirectory(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mkv")
	original := filepath.Join(dir, strings.Repeat("长", 80)+"E01.mkv")
	writeVersion(t, original, "older")
	writeVersion(t, source, "newer")
	path, err := publishVersion(context.Background(), source, original, "other version")
	if err != nil || path == original || !strings.HasSuffix(path, ".mkv") {
		t.Fatalf("长文件名的版本发布失败 %q: %v", path, err)
	}
	for path, want := range map[string]string{original: "older", path: "newer"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%q 内容 %q (%v)", path, got, err)
		}
	}
}

func TestVersionConflictPreservesBothExistingTargets(t *testing.T) {
	dir := t.TempDir()
	src, base := filepath.Join(dir, "source.mkv"), filepath.Join(dir, "E01.mkv")
	writeVersion(t, src, "newer")
	writeVersion(t, base, "older")
	other, _, err := versionTarget(context.Background(), src, base, "version")
	if err != nil {
		t.Fatal(err)
	}
	writeVersion(t, other, "third")
	_, err = publishVersion(context.Background(), src, base, "version")
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("两个候选目标都被占用，期望可复核的冲突: %v", err)
	}
	for path, want := range map[string]string{src: "newer", base: "older", other: "third"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s 冲突后内容 %q (%v)", path, got, err)
		}
	}
}

func TestCrossVolumeVersionRetryReusesPreviousCopy(t *testing.T) {
	dir, err := os.MkdirTemp("/dev/shm", "anigo-version-test-")
	if err != nil {
		t.Skip("没有可用的第二个文件系统")
	}
	defer os.RemoveAll(dir)
	src, dst := filepath.Join(t.TempDir(), "source.mkv"), filepath.Join(dir, "E01.mkv")
	writeVersion(t, src, "newer")
	if err := os.Link(src, filepath.Join(dir, "probe")); err == nil {
		t.Skip("测试需要两个独立的文件系统")
	}
	writeVersion(t, dst, "older")
	alt, err := publishVersion(context.Background(), src, dst, "cross-volume")
	if err != nil || alt == dst {
		t.Fatalf("跨卷发布版本 %s: %v", alt, err)
	}
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	retry, err := publishVersion(context.Background(), src, dst, "cross-volume")
	if err != nil || retry != alt {
		t.Fatalf("重试应复用跨卷副本 %s，实际为 %s (%v)", alt, retry, err)
	}
	if got, err := os.ReadFile(retry); err != nil || string(got) != "newer" {
		t.Fatalf("目标副本内容 %q (%v)", got, err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("重试产生了多余的默认版本: %v", err)
	}
}
