package source

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestNyaaHTTPStatusReturnsBeforeErrorPageCompletes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, strings.Repeat("Maintenance. ", 128))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := NewNyaaSource("").FetchRSS(ctx, server.URL)
	if err == nil || !strings.Contains(err.Error(), "503") || ctx.Err() != nil {
		t.Fatalf("收到 HTTP 503 应立即返回状态错误：err=%v ctx=%v", err, ctx.Err())
	}
}

func TestConcurrentSourceSearchesBoundActiveRequestsAndHonorCancellation(t *testing.T) {
	var active, peak, started atomic.Int32
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); p < n; p = peak.Load() {
			if peak.CompareAndSwap(p, n) {
				break
			}
		}
		started.Add(1)
		select {
		case <-r.Context().Done():
		case <-release:
			io.WriteString(w, "<rss><channel/></rss>")
		}
	}))
	defer server.Close()
	domain := strings.TrimPrefix(server.URL, "https://")
	n, a, at := NewNyaaSource(domain), NewACGRIPSource(domain), NewAnimeToshoSource(domain)
	n.httpClient, a.httpClient, at.httpClient = server.Client(), server.Client(), server.Client()
	ms := NewMultiSource(n, a, at)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ms.SearchAnime(ctx, "Fixture")
			if !errors.Is(err, context.Canceled) {
				t.Errorf("取消后搜索返回 %v", err)
			}
		}()
	}
	deadline := time.Now().Add(time.Second)
	for started.Load() < 8 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()
	close(release)
	if peak.Load() != 8 || started.Load() != 8 {
		t.Errorf("跨搜索实际并发 %d，请求数 %d，希望最多 8 个，其余在取消后不再请求", peak.Load(), started.Load())
	}
}

func TestRSSParsingFailureAndCancellationAllowLaterPoll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/broken":
			io.WriteString(w, "<rss><channel><item></channel>")
		case "/wait":
			io.WriteString(w, "<rss><channel>")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			io.WriteString(w, "<rss><channel><item><title>Fixture</title><enclosure url=\"https://fixture.example/f.torrent\"/></item></channel></rss>")
		}
	}))
	defer server.Close()
	sources := []core.Source{NewMikanSource("", "", nil), NewNyaaSource(""), NewACGRIPSource(""), NewAnimeToshoSource("")}
	for _, src := range sources {
		t.Run(src.Name(), func(t *testing.T) {
			if _, err := src.FetchRSS(context.Background(), server.URL+"/broken"); err == nil {
				t.Error("截断的 RSS 应返回错误")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			_, err := src.FetchRSS(ctx, server.URL+"/wait")
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("慢速响应取消错误 %v", err)
			}
			items, err := src.FetchRSS(context.Background(), server.URL+"/retry")
			if err != nil || len(items) != 1 || items[0].Title != "Fixture" {
				t.Fatalf("后续轮询失败 items=%+v err=%v", items, err)
			}
		})
	}
}
