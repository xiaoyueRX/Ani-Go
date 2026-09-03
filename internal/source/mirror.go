// Package source 实现各资源站点的 Source 接口
// Mikan 镜像源管理与测速
package source

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

// ============================================================
// 镜像延迟测试结果
// ============================================================

// MirrorLatency 镜像延迟测试结果
type MirrorLatency struct {
	Domain  string `json:"domain"`
	Latency int64  `json:"latency_ms"` // 毫秒
	OK      bool   `json:"ok"`
}

// TestLatency 并发测试所有镜像延迟，返回结果（不改变内部状态）
func (m *MikanSource) TestLatency(ctx context.Context) []MirrorLatency {
	m.mu.RLock()
	domain := m.domain
	mirrorDomains := m.mirrorDomains
	m.mu.RUnlock()

	domains := make([]string, 0, 1+len(mirrorDomains))
	domains = append(domains, domain)
	domains = append(domains, mirrorDomains...)

	// 去重
	seen := make(map[string]bool)
	unique := make([]string, 0, len(domains))
	for _, d := range domains {
		if !seen[d] {
			seen[d] = true
			unique = append(unique, d)
		}
	}

	results := make([]MirrorLatency, len(unique))
	var wg sync.WaitGroup
	for i, domain := range unique {
		wg.Add(1)
		go func(idx int, d string) {
			defer wg.Done()
			start := time.Now()
			url := fmt.Sprintf("https://%s/", d)
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
			if err != nil {
				results[idx] = MirrorLatency{Domain: d, Latency: 99999, OK: false}
				return
			}
			req.Header.Set("User-Agent", "Mozilla/5.0")
			resp, err := (httpx.New(8 * time.Second)).Do(req)
			elapsed := time.Since(start).Milliseconds()
			if err != nil {
				results[idx] = MirrorLatency{Domain: d, Latency: elapsed, OK: false}
				return
			}
			resp.Body.Close()
			results[idx] = MirrorLatency{Domain: d, Latency: elapsed, OK: true}
		}(i, domain)
	}
	wg.Wait()
	return results
}

// BestDomain 从延迟结果中选择最快的域名
func BestDomain(results []MirrorLatency, fallback string) string {
	best := fallback
	bestLatency := int64(99999)
	for _, r := range results {
		if r.OK && r.Latency < bestLatency {
			bestLatency = r.Latency
			best = r.Domain
		}
	}
	return best
}

// tryMirrors 依次尝试通过代理域名、主域名、镜像域名发起 HTTP GET 请求
// 在 GFW 环境下主域名可能不可达，自动回退到镜像域名
func (m *MikanSource) tryMirrors(ctx context.Context, path string) (*http.Response, error) {
	m.mu.RLock()
	domain := m.domain
	mirrorDomains := m.mirrorDomains
	m.mu.RUnlock()

	domains := make([]string, 0, 1+len(mirrorDomains))
	domains = append(domains, domain)
	domains = append(domains, mirrorDomains...)

	var lastErr error
	for _, domain := range domains {
		url := fmt.Sprintf("https://%s%s", domain, path)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

		resp, err := m.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		resp.Body.Close()
		lastErr = fmt.Errorf("镜像 %s 返回状态码: %d", domain, resp.StatusCode)
	}
	return nil, fmt.Errorf("所有镜像均不可达: %w", lastErr)
}
