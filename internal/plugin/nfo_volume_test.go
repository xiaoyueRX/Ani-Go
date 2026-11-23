package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

func markedLibrary(t *testing.T) (library, source string) {
	t.Helper()
	home := t.TempDir()
	library, source = filepath.Join(home, "Library"), filepath.Join(home, "source.mkv")
	if err := os.Mkdir(library, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(library, ".anigo-volume-id"), []byte("sidecar-disk"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("synthetic video"), 0600); err != nil {
		t.Fatal(err)
	}
	return library, source
}

func swapLibrary(t *testing.T, library string) string {
	t.Helper()
	online := library + "-original"
	if err := os.Rename(library, online); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(library, 0700); err != nil {
		t.Fatal(err)
	}
	return online
}

func assertEmptyLibrary(t *testing.T, dir string) {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("替代媒体库被写入: %v, %v", files, err)
	}
}

func TestLatePosterKeepsReplacementVolumeEmpty(t *testing.T) {
	t.Setenv("TV_VOLUME_ID", "sidecar-disk")
	library, source := markedLibrary(t)
	bus := event.New()
	t.Cleanup(func() { bus.Close(t.Context()) })
	p := NewNFOScraperPlugin()
	p.allowPrivateForTest = true
	if err := p.Init(bus, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Stop(bus) })
	// 延迟海报响应，先让真实整理器完成剧集入库和 NFO 生成。
	started, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("synthetic poster"))
	}))
	defer server.Close()
	defer unblock()
	org := organizer.New("Series/Season 1/E01.mkv", "", "", library, "", true, bus)
	_, err := org.Organize(t.Context(), source, core.Anime{TitleCN: "Series", Type: "tv", CoverURL: server.URL}, core.Episode{Season: 1, Number: 1})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("没有收到海报请求")
	}
	online := swapLibrary(t, library)
	unblock()
	bus.Wait()
	assertEmptyLibrary(t, library)
	if b, err := os.ReadFile(filepath.Join(online, "Series", "Season 1", "E01.mkv")); err != nil || string(b) != "synthetic video" {
		t.Fatalf("正确卷的媒体文件异常: %v", err)
	}
}

func TestNFOUsesOriginalVolumeAfterEventDelivery(t *testing.T) {
	for _, tc := range []struct {
		kind, key string
		files     []string
	}{
		{"tv", "TV_VOLUME_ID", []string{"Series/Season 1/E01.nfo", "Series/tvshow.nfo", "Series/poster.jpg"}},
		{"movie", "MOVIE_VOLUME_ID", []string{"Film/movie.nfo", "Film/poster.jpg"}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Setenv(tc.key, "sidecar-disk")
			library, source := markedLibrary(t)
			producer, consumer := newTestBus(), newTestBus()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "image/jpeg")
				w.Write([]byte("poster"))
			}))
			defer server.Close()
			p := NewNFOScraperPlugin()
			p.allowPrivateForTest = true
			if err := p.Init(consumer, nil); err != nil {
				t.Fatal(err)
			}
			defer p.Stop(consumer)
			var captured core.Event
			producer.Subscribe(core.EventFileOrganized, func(e core.Event) { captured = e })
			org := organizer.New("Series/Season 1/E01.mkv", "Film/Film.mkv", "", library, library, true, producer)
			_, err := org.Organize(t.Context(), source, core.Anime{TitleCN: "Test", Type: tc.kind, CoverURL: server.URL}, core.Episode{Season: 1, Number: 1})
			if err != nil {
				t.Fatal(err)
			}
			serialized, err := json.Marshal(captured)
			if err != nil || strings.Contains(string(serialized), "sidecar-disk") {
				t.Fatalf("外部事件序列化异常: %s %v", serialized, err)
			}
			online := swapLibrary(t, library)
			consumer.Publish(captured)
			assertEmptyLibrary(t, library)
			if requests.Load() != 0 {
				t.Fatal("卷已离线仍然请求了海报")
			}
			if err := os.Remove(library); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(online, library); err != nil {
				t.Fatal(err)
			}
			consumer.Publish(captured)
			if requests.Load() != 1 {
				t.Fatalf("挂回后的海报请求数 %d", requests.Load())
			}
			for _, name := range tc.files {
				if b, err := os.ReadFile(filepath.Join(library, filepath.FromSlash(name))); err != nil || len(b) == 0 {
					t.Fatalf("挂回后缺少 %s: %v", name, err)
				}
			}
		})
	}
}

func TestNFOChecksVolumeAgainBeforePublishingTemporaryFile(t *testing.T) {
	t.Setenv("TV_VOLUME_ID", "sidecar-disk")
	library, source := markedLibrary(t)
	bus := newTestBus()
	var ev core.Event
	bus.Subscribe(core.EventFileOrganized, func(e core.Event) { ev = e })
	org := organizer.New("Series/E01.mkv", "", "", library, "", true, bus)
	_, err := org.Organize(t.Context(), source, core.Anime{Type: "tv"}, core.Episode{Season: 1, Number: 1})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(library, "Series", "test.nfo")
	dir := filepath.Dir(target)
	changed := false
	writeDir := func(path string) (*os.Root, error) {
		temps, err := filepath.Glob(filepath.Join(dir, ".anigo-nfo-*"))
		if err != nil {
			return nil, err
		}
		if len(temps) > 0 {
			changed = true
			if err := os.WriteFile(filepath.Join(library, ".anigo-volume-id"), []byte("other"), 0600); err != nil {
				return nil, err
			}
		}
		return ev.OpenLibraryDir(path)
	}
	if err := writeSidecar(target, []byte("metadata"), writeDir); err == nil || !changed {
		t.Fatalf("发布前卷标发生变化: changed=%v, err=%v", changed, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("副文件被发布: %v", err)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".anigo-nfo-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("临时文件未清理: %v, %v", temps, err)
	}
}
