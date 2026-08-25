// Package httpx 提供全局共享的 HTTP 客户端与连接池
// 优化点：此前各模块（下载器/资源站/通知器/AI/元数据）均各自新建 http.Client，
// 每个 Client 默认持有独立 Transport 连接池，空闲连接无法跨模块复用，
// 造成重复 TCP/TLS 握手的 CPU 开销与多余的内存占用。
// 现统一共享同一个 Transport，Client 结构体本身极轻量（仅数个字段），
// 昂贵的连接建立成本由全进程摊薄。
package httpx

import (
	"net"
	"net/http"
	"time"
)

// sharedTransport 全局唯一连接池
// 参数依据 OPTIMIZE_TASK.md：MaxIdleConnsPerHost=10、IdleConnTimeout=90s
var sharedTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   10,             // 每个 host 保底 10 条空闲连接，覆盖 RSS/元数据/通知并发场景
	IdleConnTimeout:       90 * time.Second, // 空闲连接 90s 后回收，兼顾复用率与 fd 占用
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// Default 全局默认客户端（30s 超时），供无特殊超时需求的调用方直接使用
var Default = New(30 * time.Second)

// New 创建复用全局连接池的 HTTP 客户端
// 保留 per-client Timeout 语义（各模块超时需求不同：通知 10s / RSS 30s / Ollama 120s），
// 由 context 取代的方案会侵入所有调用链，此处不做
func New(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: sharedTransport,
	}
}
