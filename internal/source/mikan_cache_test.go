package source

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestMikanSource_ScheduleAndSubgroupCacheAndDomainIsolation(t *testing.T) {
	var scheduleHits atomic.Int32
	var subgroupHits atomic.Int32
	var returnEmpty atomic.Bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Home/BangumiCoverFlowByDayOfWeek":
			scheduleHits.Add(1)
			if returnEmpty.Load() {
				_, _ = w.Write([]byte(`<html><body></body></html>`))
				return
			}
			_, _ = w.Write([]byte(`<html><body>
				<div class="sk-bangumi" data-dayofweek="1">
					<ul><li>
						<span class="b-lazy" data-src="/cover.jpg"></span>
						<a class="an-text" href="/Home/Bangumi/101">Test Anime</a>
					</li></ul>
				</div>
			</body></html>`))
		case "/Home/Bangumi/101":
			subgroupHits.Add(1)
			_, _ = w.Write([]byte(`<html><body>
				<div class="leftbar-item"><a class="subgroup-name" data-anchor="#201">SubA</a></div>
				<div id="201"><a class="mikan-rss" href="/RSS/Bangumi?bangumiId=101&subgroupid=201"></a></div>
			</body></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	srv1 := httptest.NewServer(handler)
	defer srv1.Close()
	srv2 := httptest.NewServer(handler)
	defer srv2.Close()

	host1 := srv1.Listener.Addr().String()
	host2 := srv2.Listener.Addr().String()

	m := NewMikanSource("primary.local", "", []string{"mirror.local"})
	testClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			cloned := req.Clone(req.Context())
			cloned.URL.Scheme = "http"
			if req.URL.Host == "mirror.local" {
				cloned.URL.Host = host2
			} else {
				cloned.URL.Host = host1
			}
			return http.DefaultTransport.RoundTrip(cloned)
		}),
	}
	m.httpClient = testClient
	m.mirrorClient = testClient

	// 1. 空结果不写缓存
	returnEmpty.Store(true)
	emptyDays, err := m.FetchWeekSchedule(t.Context(), 2026, 4)
	if err != nil || len(emptyDays) != 0 {
		t.Fatalf("空季度响应异常: len=%d err=%v", len(emptyDays), err)
	}
	returnEmpty.Store(false)

	// 2. 首次有效抓取走网络，二次命中缓存
	days1, err := m.FetchWeekSchedule(t.Context(), 2026, 4)
	if err != nil || len(days1) != 1 || len(days1[0].Items) != 1 {
		t.Fatalf("首次获取时间表异常: %+v err=%v", days1, err)
	}
	if scheduleHits.Load() != 2 {
		t.Fatalf("期望累计 2 次时间表请求，实际 %d", scheduleHits.Load())
	}
	// 模拟调用方原地修改返回项，校验不污染缓存
	days1[0].Items[0].InfoHash = "subscribed"

	days2, err := m.FetchWeekSchedule(t.Context(), 2026, 4)
	if err != nil || len(days2) != 1 {
		t.Fatalf("命中缓存获取时间表异常: %v", err)
	}
	if scheduleHits.Load() != 2 {
		t.Fatalf("二次调用应命中缓存，请求数应仍为 2，实际 %d", scheduleHits.Load())
	}
	if days2[0].Items[0].InfoHash != "" {
		t.Fatalf("缓存数据被外部修改污染: InfoHash=%q", days2[0].Items[0].InfoHash)
	}

	// 3. 字幕组缓存与域名隔离
	groups1, err := m.FetchSubgroups(t.Context(), "101")
	if err != nil || len(groups1) != 1 {
		t.Fatalf("首次获取字幕组异常: %+v err=%v", groups1, err)
	}
	groups1[0].Name = "Mutated"

	groups2, err := m.FetchSubgroups(t.Context(), "101")
	if err != nil || len(groups2) != 1 || groups2[0].Name != "SubA" {
		t.Fatalf("字幕组缓存命中或深拷贝异常: %+v err=%v", groups2, err)
	}
	if subgroupHits.Load() != 1 {
		t.Fatalf("字幕组二次调用应命中缓存，请求数应为 1，实际 %d", subgroupHits.Load())
	}

	// 切换主域名后应按新域名重新拉取
	m.SetDomain("mirror.local")
	_, err = m.FetchSubgroups(t.Context(), "101")
	if err != nil || subgroupHits.Load() != 2 {
		t.Fatalf("切换域名后应失效旧缓存重新抓取，hits=%d err=%v", subgroupHits.Load(), err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
