package organizer

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type stopDuringCopy struct {
	context.Context
	dir, checkpoint string
}

func (c stopDuringCopy) Err() error {
	files, _ := filepath.Glob(filepath.Join(c.dir, stagePrefix+"*", "media"))
	for _, f := range files {
		if stat, err := os.Stat(f); err == nil && stat.Size() >= 32<<10 {
			if err := os.WriteFile(c.checkpoint, []byte(filepath.Dir(f)), 0600); err != nil {
				return err
			}
			for {
				time.Sleep(time.Second)
			}
		}
	}
	return nil
}

func TestKilledCrossVolumeCopyReclaimsOnlyAbandonedStage(t *testing.T) {
	const ctl = "ANIGO_TEST_STAGE_DIR"
	if dir := os.Getenv(ctl); dir != "" {
		ctx := stopDuringCopy{context.Background(), dir, os.Getenv("ANIGO_TEST_STAGE_SIGNAL")}
		t.Fatal(installMedia(ctx, os.Getenv("ANIGO_TEST_STAGE_SOURCE"), filepath.Join(dir, "episode.mkv")))
	}
	lib, err := os.MkdirTemp("/dev/shm", "anigo-stage-test-")
	if err != nil {
		t.Skip("no second filesystem")
	}
	defer os.RemoveAll(lib)
	probe, err := os.Open(lib)
	if err != nil {
		t.Fatal(err)
	}
	locked := lockStageDirectory(probe)
	_ = probe.Close()
	if !locked {
		t.Skip("filesystem does not lock directories")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	data := bytes.Repeat([]byte("video"), 256<<10)
	if err := os.WriteFile(src, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(src, filepath.Join(lib, "probe")); err == nil {
		t.Skip("same filesystem")
	}
	dst := filepath.Join(lib, "episode.mkv")
	if err := os.WriteFile(dst, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	live, release, err := newMediaStage(lib, lockStageDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := os.WriteFile(filepath.Join(live, "media"), []byte("active writer"), 0600); err != nil {
		t.Fatal(err)
	}
	user := filepath.Join(lib, stagePrefix+"123")
	if err := os.Mkdir(user, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, "media"), []byte("private file"), 0600); err != nil {
		t.Fatal(err)
	}
	checkpoint := filepath.Join(dir, "ready")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestKilledCrossVolumeCopyReclaimsOnlyAbandonedStage$")
	child.Env = append(os.Environ(), ctl+"="+lib, "ANIGO_TEST_STAGE_SOURCE="+src, "ANIGO_TEST_STAGE_SIGNAL="+checkpoint)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	var abandoned []byte
	for ctx.Err() == nil {
		if abandoned, err = os.ReadFile(checkpoint); err == nil && len(abandoned) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = child.Process.Kill()
	_ = child.Wait()
	if ctx.Err() != nil {
		t.Fatal("未到达跨卷复制的中断位置")
	}
	if before, err := os.ReadFile(dst); err != nil || string(before) != "original" {
		t.Fatal("半成品替换了原媒体")
	}
	if err := installMedia(context.Background(), src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(string(abandoned)); !os.IsNotExist(err) {
		t.Fatalf("遗留暂存目录: %v", err)
	}
	for path, want := range map[string][]byte{dst: data, src: data, filepath.Join(live, "media"): []byte("active writer"), filepath.Join(user, "media"): []byte("private file")} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("错误修改文件: %s %v", path, err)
		}
	}
}

func TestStagingWithoutDirectoryLocksLeavesOtherWorkAlone(t *testing.T) {
	dir := t.TempDir()
	unsupported := func(*os.File) bool { return false }
	previous, closePrevious, err := newMediaStage(dir, unsupported)
	if err != nil {
		t.Fatal(err)
	}
	defer closePrevious()
	data := []byte("possibly still in use")
	if err := os.WriteFile(filepath.Join(previous, "media"), data, 0600); err != nil {
		t.Fatal(err)
	}
	current, closeCurrent, err := newMediaStage(dir, unsupported)
	if err != nil {
		t.Fatal(err)
	}
	if current == previous {
		t.Fatal("shared staging on unsupported filesystem")
	}
	closeCurrent()
	if content, err := os.ReadFile(filepath.Join(previous, "media")); err != nil || !bytes.Equal(content, data) {
		t.Fatal("removed another stage")
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatal("own normal cleanup failed")
	}
}
