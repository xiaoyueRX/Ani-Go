package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
	"gorm.io/gorm"
)

func TestServer_Auth(t *testing.T) {
	token := "mcp-test-token-456"
	s := NewServer(func() string {
		return token
	})

	// 1. Bearer Header - 成功
	req1 := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	req1.Header.Set("Authorization", "Bearer "+token)
	if !s.VerifyAuth(req1) {
		t.Errorf("VerifyAuth 应该通过合法 Bearer Token")
	}

	// 2. Bearer Header (大小写不敏感) - 成功
	req2 := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	req2.Header.Set("Authorization", "bearer "+token)
	if !s.VerifyAuth(req2) {
		t.Errorf("VerifyAuth 应该通过大小写不敏感的 bearer Token")
	}

	// 3. Query 参数 - 成功
	req3 := httptest.NewRequest(http.MethodGet, "/mcp/sse?token="+token, nil)
	if !s.VerifyAuth(req3) {
		t.Errorf("VerifyAuth 应该通过 URL Query Token")
	}

	// 4. 缺少 Token - 失败
	req4 := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	if s.VerifyAuth(req4) {
		t.Errorf("VerifyAuth 缺少 Token 时应该失败")
	}

	// 5. 错误 Token - 失败
	req5 := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	req5.Header.Set("Authorization", "Bearer wrong-token")
	if s.VerifyAuth(req5) {
		t.Errorf("VerifyAuth 错误 Token 时应该失败")
	}

	// 6. 服务端未配置 Token 时，拒绝访问
	sEmpty := NewServer(func() string {
		return ""
	})
	req6 := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	req6.Header.Set("Authorization", "Bearer "+token)
	if sEmpty.VerifyAuth(req6) {
		t.Errorf("服务端未配置 Token 时应拒绝未授权请求")
	}
}

func TestServer_ToolRegistrationAndPanicRecovery(t *testing.T) {
	s := NewServer(func() string { return "token" })

	// 注册普通工具
	s.RegisterTool(Tool{
		Name:        "echo",
		Description: "echoes input",
		InputSchema: InputSchema{Type: "object"},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		val, _ := args["msg"].(string)
		return TextResult("echo: " + val), nil
	})

	// 注册会 panic 的危险工具，测试沙箱隔离与 panic recovery
	s.RegisterTool(Tool{
		Name:        "panic_tool",
		Description: "tool that panics",
		InputSchema: InputSchema{Type: "object"},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		panic("boom! simulated crash")
	})

	tools := s.ListTools()
	if len(tools) != 2 {
		t.Fatalf("期望 2 个工具，实际 %d 个", len(tools))
	}

	// 调用 echo 工具
	ctx := context.Background()
	res, err := s.CallTool(ctx, "echo", map[string]interface{}{"msg": "hello"})
	if err != nil {
		t.Fatalf("CallTool 失败: %v", err)
	}
	if len(res.Content) == 0 || res.Content[0].Text != "echo: hello" {
		t.Errorf("echo 输出不符合预期: %+v", res)
	}

	// 调用 panic 工具：应被 defer recover 捕获，返回 isError: true，不能导致主进程崩溃
	resPanic, errPanic := s.CallTool(ctx, "panic_tool", nil)
	if errPanic != nil {
		t.Fatalf("CallTool 不应直接抛出错误: %v", errPanic)
	}
	if !resPanic.IsError {
		t.Errorf("panic 工具应该标记为 IsError=true")
	}
	if !strings.Contains(resPanic.Content[0].Text, "panic") {
		t.Errorf("错误信息应包含 panic 提示: %s", resPanic.Content[0].Text)
	}

	// 注销工具
	s.UnregisterTool("echo")
	if len(s.ListTools()) != 1 {
		t.Errorf("注销工具后应剩余 1 个工具")
	}
}

func TestServer_SSEAndJSONRPCWorkflow(t *testing.T) {
	token := "test-sse-token"
	s := NewServer(func() string { return token })

	s.RegisterTool(Tool{
		Name:        "ping_calc",
		Description: "test calculator",
		InputSchema: InputSchema{Type: "object"},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		return TextResult("pong"), nil
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /mcp/sse", s.HandleSSE)
	mux.HandleFunc("POST /mcp/messages", s.HandleMessages)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 1. 发起 SSE 连接
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/mcp/sse", nil)
	if err != nil {
		t.Fatalf("创建 SSE 请求失败: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("发起 SSE 连接失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE 连接状态码错误: %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)

	// 读取首条 SSE 事件: event: endpoint
	var sessionEndpoint string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("读取 SSE 流失败: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: /mcp/messages?session_id=") {
			sessionEndpoint = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	if sessionEndpoint == "" {
		t.Fatalf("未接收到 session endpoint")
	}

	// 2. 发送 initialize 请求
	initReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "req-1",
		Method:  "initialize",
	}
	initBody, _ := json.Marshal(initReq)

	postReq, _ := http.NewRequest(http.MethodPost, ts.URL+sessionEndpoint, bytes.NewReader(initBody))
	postReq.Header.Set("Authorization", "Bearer "+token)
	postReq.Header.Set("Content-Type", "application/json")

	postResp, err := client.Do(postReq)
	if err != nil {
		t.Fatalf("POST /mcp/messages 失败: %v", err)
	}
	if postResp.StatusCode != http.StatusAccepted {
		t.Fatalf("期望状态码 202 Accepted，实际: %d", postResp.StatusCode)
	}
	postResp.Body.Close()

	// 3. 从 SSE 接收响应
	var jsonResp JSONRPCResponse
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("读取 SSE 响应失败: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if err := json.Unmarshal([]byte(data), &jsonResp); err == nil && jsonResp.ID == "req-1" {
				break
			}
		}
	}

	if jsonResp.Error != nil {
		t.Fatalf("initialize 响应报错: %+v", jsonResp.Error)
	}

	// 4. 调用 tools/call
	callReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "req-2",
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"ping_calc","arguments":{}}`),
	}
	callBody, _ := json.Marshal(callReq)

	postReq2, _ := http.NewRequest(http.MethodPost, ts.URL+sessionEndpoint, bytes.NewReader(callBody))
	postReq2.Header.Set("Authorization", "Bearer "+token)
	postReq2.Header.Set("Content-Type", "application/json")

	postResp2, err := client.Do(postReq2)
	if err != nil {
		t.Fatalf("POST tools/call 失败: %v", err)
	}
	postResp2.Body.Close()

	var callResp JSONRPCResponse
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("读取 SSE 响应失败: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if err := json.Unmarshal([]byte(data), &callResp); err == nil && callResp.ID == "req-2" {
				break
			}
		}
	}

	if callResp.Error != nil {
		t.Fatalf("tools/call 报错: %+v", callResp.Error)
	}
}

// 模拟 Downloader
type mockDownloader struct{}

func (m *mockDownloader) Name() string { return "mock_qb" }
func (m *mockDownloader) Add(ctx context.Context, item core.TorrentItem, savePath string) error {
	return nil
}
func (m *mockDownloader) List(ctx context.Context) ([]core.DownloadTask, error) {
	return []core.DownloadTask{
		{
			Hash:      "hash123",
			Name:      "[Test] Frieren - 01.mp4",
			Progress:  0.85,
			SpeedDown: 1024 * 1024 * 5, // 5 MB/s
			Status:    "downloading",
		},
	}, nil
}
func (m *mockDownloader) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	return core.DownloadTask{}, nil
}
func (m *mockDownloader) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	return nil
}
func (m *mockDownloader) Pause(ctx context.Context, hash string) error  { return nil }
func (m *mockDownloader) Resume(ctx context.Context, hash string) error { return nil }
func (m *mockDownloader) IsAvailable(ctx context.Context) bool          { return true }

func TestGovernanceAndAnimeTools(t *testing.T) {
	// 初始化内存 SQLite 测试库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}
	database.DB = db
	_ = db.AutoMigrate(&database.Setting{}, &database.Subscription{}, &database.Episode{})

	s := NewServer(func() string { return "token" })

	// 测试 Governance Tools
	mockBus := &mockBus{}
	pm := plugin.NewManager(mockBus)
	s.RegisterGovernanceTools(GovernanceDeps{
		PluginManager: pm,
		Downloader:    &mockDownloader{},
		Version:       "v0.6.0",
	})

	ctx := context.Background()

	// 1. list_plugins
	res, err := s.CallTool(ctx, "list_plugins", map[string]interface{}{})
	if err != nil || res.IsError {
		t.Fatalf("list_plugins 失败: %v, %+v", err, res)
	}
	if !strings.Contains(res.Content[0].Text, "插件列表") {
		t.Errorf("list_plugins 输出不符合预期: %s", res.Content[0].Text)
	}

	// 2. get_system_status
	resStatus, err := s.CallTool(ctx, "get_system_status", map[string]interface{}{})
	if err != nil || resStatus.IsError {
		t.Fatalf("get_system_status 失败: %v, %+v", err, resStatus)
	}
	if !strings.Contains(resStatus.Content[0].Text, "Ani-Go 系统健康报告") {
		t.Errorf("get_system_status 输出不符合预期: %s", resStatus.Content[0].Text)
	}

	// 3. update_system_config
	resConfig, err := s.CallTool(ctx, "update_system_config", map[string]interface{}{
		"key":   "TEST_CONFIG_KEY",
		"value": "12345",
	})
	if err != nil || resConfig.IsError {
		t.Fatalf("update_system_config 失败: %v", err)
	}

	// 测试 AnimeOps Tools
	var refreshed atomic.Bool
	s.RegisterAnimeOpsTools(AnimeOpsDeps{
		Downloader: &mockDownloader{},
		TriggerRefresh: func() {
			refreshed.Store(true)
		},
	})

	// get_downloads
	resDL, err := s.CallTool(ctx, "get_downloads", map[string]interface{}{"status": "all"})
	if err != nil || resDL.IsError {
		t.Fatalf("get_downloads 失败: %v", err)
	}
	if !strings.Contains(resDL.Content[0].Text, "Frieren") {
		t.Errorf("get_downloads 输出未包含任务: %s", resDL.Content[0].Text)
	}

	// trigger_refresh
	resRef, err := s.CallTool(ctx, "trigger_refresh", map[string]interface{}{})
	if err != nil || resRef.IsError {
		t.Fatalf("trigger_refresh 失败: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if !refreshed.Load() {
		t.Errorf("trigger_refresh 未触发回调函数")
	}
}

type mockBus struct{}

func (m *mockBus) Publish(event core.Event) {}
func (m *mockBus) Subscribe(eventType string, handler core.EventHandler) core.SubscriptionID {
	return 1
}
func (m *mockBus) Unsubscribe(eventType string, id core.SubscriptionID) {}

type nonFlusherWriter struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (w *nonFlusherWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *nonFlusherWriter) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *nonFlusherWriter) WriteHeader(code int)        { w.code = code }

type mockSource struct {
	items []core.TorrentItem
	err   error
}

func (m *mockSource) Name() string { return "mock_multi" }
func (m *mockSource) FetchRSS(ctx context.Context, rssURL string) ([]core.TorrentItem, error) {
	return nil, nil
}
func (m *mockSource) FetchLatest(ctx context.Context) ([]core.TorrentItem, error) {
	return nil, nil
}
func (m *mockSource) SearchAnime(ctx context.Context, keyword string) ([]core.TorrentItem, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.items, nil
}
func (m *mockSource) FetchHistory(ctx context.Context, bangumiID string, filter core.Filter) ([]core.TorrentItem, error) {
	return nil, nil
}
func (m *mockSource) IsAvailable(ctx context.Context) bool {
	return true
}

type failingDownloader struct {
	mockDownloader
	err error
}

func (f *failingDownloader) List(ctx context.Context) ([]core.DownloadTask, error) {
	return nil, f.err
}

func TestServer_AuthEdgeCases(t *testing.T) {
	const token = "secret-mcp-token-xyz"
	s := NewServer(func() string { return "  " + token + "  " })

	if s.VerifyAuth(nil) {
		t.Errorf("nil request 应该被拒绝")
	}

	sNilGetter := NewServer(nil)
	reqValid := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	reqValid.Header.Set("Authorization", "Bearer "+token)
	if sNilGetter.VerifyAuth(reqValid) {
		t.Errorf("nil tokenGetter 应该被拒绝")
	}

	sWhitespace := NewServer(func() string { return "   \t\n  " })
	if sWhitespace.VerifyAuth(reqValid) {
		t.Errorf("仅包含空白字符的 token 配置应该被拒绝")
	}

	cases := []struct {
		name   string
		header string
		query  string
		want   bool
	}{
		{name: "空 Header 与 Query", header: "", query: "", want: false},
		{name: "仅 Bearer 无内容", header: "Bearer", want: false},
		{name: "Bearer 后只有空格", header: "Bearer    ", want: false},
		{name: "Bearer 含多余段", header: "Bearer " + token + " extra", want: false},
		{name: "错误前缀 Basic", header: "Basic " + token, want: false},
		{name: "前缀匹配但长度更长", header: "Bearer " + token + "1", want: false},
		{name: "前缀匹配但长度更短", header: "Bearer " + token[:len(token)-1], want: false},
		{name: "Tab 分隔 Bearer", header: "Bearer\t" + token, want: true},
		{name: "多空格分隔 Bearer", header: "Bearer   " + token, want: true},
		{name: "Query 空 token", query: "token=", want: false},
		{name: "Query 空白 token", query: "token=%20%20", want: false},
		{name: "Query 合法 token", query: "token=" + token, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := "/mcp/sse"
			if tc.query != "" {
				target += "?" + tc.query
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if got := s.VerifyAuth(req); got != tc.want {
				t.Errorf("VerifyAuth(%q, %q) = %v, 期望 %v", tc.header, tc.query, got, tc.want)
			}
		})
	}
}

func TestServer_SSEEdgeCasesAndEviction(t *testing.T) {
	const token = "edge-token"
	s := NewServer(func() string { return token })
	s.sessionTimeout = 60 * time.Millisecond
	s.heartbeatInterval = 15 * time.Millisecond
	s.maxSessions = 2
	s.maxBodyBytes = 128

	// 1. 错误 HTTP 方法校验
	reqWrongSSE := httptest.NewRequest(http.MethodPost, "/mcp/sse", nil)
	reqWrongSSE.Header.Set("Authorization", "Bearer "+token)
	recWrongSSE := httptest.NewRecorder()
	s.HandleSSE(recWrongSSE, reqWrongSSE)
	if recWrongSSE.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /mcp/sse 期望 405，实际 %d", recWrongSSE.Code)
	}

	reqWrongMsg := httptest.NewRequest(http.MethodGet, "/mcp/messages?session_id=abc", nil)
	reqWrongMsg.Header.Set("Authorization", "Bearer "+token)
	recWrongMsg := httptest.NewRecorder()
	s.HandleMessages(recWrongMsg, reqWrongMsg)
	if recWrongMsg.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /mcp/messages 期望 405，实际 %d", recWrongMsg.Code)
	}

	// 2. 不支持 http.Flusher 的 ResponseWriter 返回 500
	reqNoFlush := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	reqNoFlush.Header.Set("Authorization", "Bearer "+token)
	nfWriter := &nonFlusherWriter{}
	s.HandleSSE(nfWriter, reqNoFlush)
	if nfWriter.code != http.StatusInternalServerError {
		t.Errorf("不支持 Flusher 时期望 500，实际 %d", nfWriter.code)
	}

	// 3. 启动真实 HTTP 服务验证 maxSessions、空闲超时淘汰、Close() 与请求体上限
	mux := http.NewServeMux()
	mux.HandleFunc("GET /mcp/sse", s.HandleSSE)
	mux.HandleFunc("POST /mcp/messages", s.HandleMessages)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	defer s.Close()

	openSSE := func() (*http.Response, string) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/mcp/sse", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("建立 SSE 失败: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("建立 SSE 状态码错误: %d", resp.StatusCode)
		}
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				resp.Body.Close()
				t.Fatalf("读取 endpoint 失败: %v", err)
			}
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data: /mcp/messages?session_id=") {
				return resp, strings.TrimPrefix(line, "data: ")
			}
		}
	}

	resp1, ep1 := openSSE()
	defer resp1.Body.Close()
	resp2, _ := openSSE()
	defer resp2.Body.Close()

	// 第 3 个会话触发 maxSessions=2 -> 503
	req3, _ := http.NewRequest(http.MethodGet, ts.URL+"/mcp/sse", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("请求第 3 个 SSE 失败: %v", err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("超过 maxSessions 期望 503，实际 %d", resp3.StatusCode)
	}

	// 请求体超过 maxBodyBytes (128B) -> 413
	hugeBody := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":"%s"}}`, strings.Repeat("a", 256))
	reqHuge, _ := http.NewRequest(http.MethodPost, ts.URL+ep1, strings.NewReader(hugeBody))
	reqHuge.Header.Set("Authorization", "Bearer "+token)
	respHuge, err := http.DefaultClient.Do(reqHuge)
	if err != nil {
		t.Fatalf("发送超大请求失败: %v", err)
	}
	respHuge.Body.Close()
	if respHuge.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("超大请求体期望 413，实际 %d", respHuge.StatusCode)
	}

	// 等待会话空闲超时并主动淘汰
	time.Sleep(110 * time.Millisecond)
	evicted := s.EvictStaleSessions()
	if evicted == 0 && s.SessionCount() != 0 {
		t.Errorf("期望过期会话被清理，evicted=%d, remaining=%d", evicted, s.SessionCount())
	}

	// 已淘汰的 session 再发消息应返回 404
	reqExpired, _ := http.NewRequest(http.MethodPost, ts.URL+ep1, strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"ping"}`))
	reqExpired.Header.Set("Authorization", "Bearer "+token)
	respExpired, err := http.DefaultClient.Do(reqExpired)
	if err != nil {
		t.Fatalf("向过期会话发送请求失败: %v", err)
	}
	respExpired.Body.Close()
	if respExpired.StatusCode != http.StatusNotFound {
		t.Errorf("向已淘汰会话发消息期望 404，实际 %d", respExpired.StatusCode)
	}
}

func TestServer_JSONRPCProtocolErrorsAndNotifications(t *testing.T) {
	const token = "rpc-token"
	s := NewServer(func() string { return token })
	s.RegisterTool(Tool{Name: "b_tool", InputSchema: InputSchema{Type: "object"}}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		return TextResult("b"), nil
	})
	s.RegisterTool(Tool{Name: "a_tool", InputSchema: InputSchema{Type: "object"}}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		return TextResult("a"), nil
	})

	// 验证 ListTools 按工具名稳定排序
	tools := s.ListTools()
	if len(tools) != 2 || tools[0].Name != "a_tool" || tools[1].Name != "b_tool" {
		t.Fatalf("ListTools 应按名称排序，实际: %+v", tools)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /mcp/sse", s.HandleSSE)
	mux.HandleFunc("POST /mcp/messages", s.HandleMessages)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	defer s.Close()

	reqSSE, _ := http.NewRequest(http.MethodGet, ts.URL+"/mcp/sse", nil)
	reqSSE.Header.Set("Authorization", "Bearer "+token)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reqSSE = reqSSE.WithContext(ctx)

	respSSE, err := http.DefaultClient.Do(reqSSE)
	if err != nil {
		t.Fatalf("SSE 连接失败: %v", err)
	}
	defer respSSE.Body.Close()

	reader := bufio.NewReader(respSSE.Body)
	var sessionEndpoint string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("读取 endpoint 失败: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: /mcp/messages?session_id=") {
			sessionEndpoint = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	postRPC := func(payload string) *http.Response {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+sessionEndpoint, strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST 失败: %v", err)
		}
		return resp
	}

	readRPCResp := func(targetID string) JSONRPCResponse {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("读取 SSE 响应失败 (等待 id=%s): %v", targetID, err)
			}
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data: ") {
				var r JSONRPCResponse
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &r); err == nil && fmt.Sprint(r.ID) == targetID {
					return r
				}
			}
		}
	}

	// 1. 畸形非对象 JSON 报文应在 HTTP 层返回 400
	for _, bad := range []string{"", "   ", "null", "[]", "123", "{bad", `{"jsonrpc":"2.0","method":"ping"}trailing`} {
		r := postRPC(bad)
		r.Body.Close()
		if r.StatusCode != http.StatusBadRequest {
			t.Errorf("畸形报文 %q 期望 400，实际 %d", bad, r.StatusCode)
		}
	}

	// 2. 通知类请求 (notifications/initialized, notifications/cancelled) 不应生成任何 SSE 响应
	for _, notif := range []string{
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled"}`,
	} {
		r := postRPC(notif)
		r.Body.Close()
		if r.StatusCode != http.StatusAccepted {
			t.Errorf("通知请求期望 202，实际 %d", r.StatusCode)
		}
	}

	// 3. 协议错误码检验：错误 jsonrpc 版本 (-32600)、空方法 (-32600)、未知方法 (-32601)、未知工具 (-32602)、空工具名 (-32602)、缺失 params (-32602)
	errCases := []struct {
		id       string
		payload  string
		wantCode int
	}{
		{"e1", `{"jsonrpc":"1.0","id":"e1","method":"ping"}`, -32600},
		{"e2", `{"jsonrpc":"2.0","id":"e2","method":"   "}`, -32600},
		{"e3", `{"jsonrpc":"2.0","id":"e3","method":"nonexistent/method"}`, -32601},
		{"e4", `{"jsonrpc":"2.0","id":"e4","method":"tools/call"}`, -32602},
		{"e5", `{"jsonrpc":"2.0","id":"e5","method":"tools/call","params":null}`, -32602},
		{"e6", `{"jsonrpc":"2.0","id":"e6","method":"tools/call","params":{"name":""}}`, -32602},
		{"e7", `{"jsonrpc":"2.0","id":"e7","method":"tools/call","params":{"name":"unknown_tool"}}`, -32602},
	}

	for _, tc := range errCases {
		r := postRPC(tc.payload)
		r.Body.Close()
		if r.StatusCode != http.StatusAccepted {
			t.Fatalf("[%s] 期望 202，实际 %d", tc.id, r.StatusCode)
		}
		rpcResp := readRPCResp(tc.id)
		if rpcResp.Error == nil || rpcResp.Error.Code != tc.wantCode {
			t.Errorf("[%s] 期望错误码 %d，实际 %+v", tc.id, tc.wantCode, rpcResp.Error)
		}
	}

	// 4. ping & tools/list 正常响应
	rPing := postRPC(`{"jsonrpc":"2.0","id":"p1","method":"ping"}`)
	rPing.Body.Close()
	if resp := readRPCResp("p1"); resp.Error != nil {
		t.Errorf("ping 不应出错: %+v", resp.Error)
	}

	rList := postRPC(`{"jsonrpc":"2.0","id":"l1","method":"tools/list"}`)
	rList.Body.Close()
	if resp := readRPCResp("l1"); resp.Error != nil {
		t.Errorf("tools/list 不应出错: %+v", resp.Error)
	}
}

func TestAllTools_ValidationNilGuardsAndSecurity(t *testing.T) {
	oldDB := database.DB
	defer func() { database.DB = oldDB }()

	ctx := context.Background()

	// 1. 验证所有依赖与 DB 为 nil 时的安全防护（不能 panic）
	sNil := NewServer(func() string { return "tok" })
	sNil.RegisterAnimeOpsTools(AnimeOpsDeps{})
	sNil.RegisterGovernanceTools(GovernanceDeps{})
	database.DB = nil

	for _, tc := range []struct {
		tool string
		args map[string]interface{}
	}{
		{"search_anime", map[string]interface{}{"keyword": "芙莉莲"}},
		{"list_subscriptions", nil},
		{"add_subscription", map[string]interface{}{"title": "芙莉莲"}},
		{"remove_subscription", map[string]interface{}{"subscription_id": float64(1)}},
		{"get_downloads", nil},
		{"trigger_refresh", nil},
		{"list_plugins", nil},
		{"toggle_plugin", map[string]interface{}{"plugin_id": "extended-notifiers", "enabled": true}},
		{"update_system_config", map[string]interface{}{"key": "TV_BASE_PATH", "value": "/tv"}},
	} {
		res, err := sNil.CallTool(ctx, tc.tool, tc.args)
		if err != nil {
			t.Fatalf("[%s] CallTool 不应返回 Go error: %v", tc.tool, err)
		}
		if !res.IsError {
			t.Errorf("[%s] 依赖未配置/DB 为 nil 时应返回 IsError=true，实际: %+v", tc.tool, res)
		}
	}

	// get_system_status 在无 DB、无 Downloader 时仍应健康返回而非报错
	resStatusNil, err := sNil.CallTool(ctx, "get_system_status", nil)
	if err != nil || resStatusNil.IsError {
		t.Fatalf("get_system_status 在无可选依赖时不应报错: %v, %+v", err, resStatusNil)
	}
	if !strings.Contains(resStatusNil.Content[0].Text, "数据库未就绪") {
		t.Errorf("期望提示数据库未就绪: %s", resStatusNil.Content[0].Text)
	}

	// 2. 初始化内存数据库测试业务与安全边界
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("初始化内存数据库失败: %v", err)
	}
	database.DB = db
	_ = db.AutoMigrate(&database.Setting{}, &database.Subscription{}, &database.Episode{})

	s := NewServer(func() string { return "tok" })
	src := &mockSource{
		items: []core.TorrentItem{
			{Title: "Item Mikan", SourceName: "Mikan", Size: 1024 * 1024 * 500, URL: "https://mikanani.me/1.torrent"},
			{Title: "Item ACGRIP", SourceName: "ACG.RIP", Size: 1024 * 1024 * 300, MagnetURL: "magnet:?xt=urn:btih:abc"},
		},
	}
	pm := plugin.NewManager(&mockBus{})
	s.RegisterAnimeOpsTools(AnimeOpsDeps{
		MultiSource:    src,
		Downloader:     &mockDownloader{},
		TriggerRefresh: func() { panic("boom in refresh") },
	})
	s.RegisterGovernanceTools(GovernanceDeps{
		PluginManager: pm,
		Downloader:    &failingDownloader{err: errors.New("connection refused")},
		Version:       "v0.6.0",
	})

	// 3. search_anime 源过滤与空关键词校验
	if res, _ := s.CallTool(ctx, "search_anime", map[string]interface{}{"keyword": "   "}); !res.IsError {
		t.Errorf("空关键词应报错")
	}
	resSearch, _ := s.CallTool(ctx, "search_anime", map[string]interface{}{"keyword": "Frieren", "source": "acgrip", "limit": float64(5)})
	if resSearch.IsError || !strings.Contains(resSearch.Content[0].Text, "Item ACGRIP") || strings.Contains(resSearch.Content[0].Text, "Item Mikan") {
		t.Errorf("search_anime source 过滤结果异常: %+v", resSearch)
	}

	// 4. add_subscription 输入校验与正常创建
	for _, badArgs := range []map[string]interface{}{
		{"title": ""},
		{"title": "Test", "season": float64(0)},
		{"title": "Test", "season": float64(1.5)},
		{"title": "Test", "season": "bad"},
		{"title": "Test", "rss_url": "file:///etc/passwd"},
		{"title": "Test", "rss_url": "not-a-url"},
	} {
		if res, _ := s.CallTool(ctx, "add_subscription", badArgs); !res.IsError {
			t.Errorf("add_subscription 非法参数 %+v 应返回错误", badArgs)
		}
	}

	for i := 1; i <= 3; i++ {
		resAdd, _ := s.CallTool(ctx, "add_subscription", map[string]interface{}{
			"title":    fmt.Sprintf("测试番剧-%d", i),
			"season":   float64(i),
			"rss_url":  fmt.Sprintf("https://mikanani.me/RSS/Bangumi?bangumiId=%d", i),
			"subgroup": "LoliHouse",
		})
		if resAdd.IsError {
			t.Fatalf("add_subscription 创建失败: %+v", resAdd)
		}
	}

	// 为订阅 ID=1 插入关联剧集以验证级联删除
	db.Create(&database.Episode{SubscriptionID: 1, Season: 1, Number: 1, TorrentHash: "hash-ep-1"})

	// 5. list_subscriptions 分页
	resListPage, _ := s.CallTool(ctx, "list_subscriptions", map[string]interface{}{"limit": float64(2), "offset": float64(1)})
	if resListPage.IsError || !strings.Contains(resListPage.Content[0].Text, "共 3 部") || !strings.Contains(resListPage.Content[0].Text, "本次展示 2-3") {
		t.Errorf("list_subscriptions 分页输出异常: %+v", resListPage)
	}
	if resBadList, _ := s.CallTool(ctx, "list_subscriptions", map[string]interface{}{"limit": float64(0)}); !resBadList.IsError {
		t.Errorf("list_subscriptions limit=0 应报错")
	}

	// 6. remove_subscription 参数校验与级联删除
	for _, badID := range []interface{}{float64(0), float64(-1), float64(1.5), "1", nil} {
		if res, _ := s.CallTool(ctx, "remove_subscription", map[string]interface{}{"subscription_id": badID}); !res.IsError {
			t.Errorf("remove_subscription 非法 ID %v 应报错", badID)
		}
	}
	if resMiss, _ := s.CallTool(ctx, "remove_subscription", map[string]interface{}{"subscription_id": float64(9999)}); !resMiss.IsError {
		t.Errorf("删除不存在订阅应报错")
	}
	if resDel, _ := s.CallTool(ctx, "remove_subscription", map[string]interface{}{"subscription_id": float64(1)}); resDel.IsError {
		t.Fatalf("删除订阅 1 失败: %+v", resDel)
	}
	var epCount int64
	db.Model(&database.Episode{}).Where("subscription_id = ?", 1).Count(&epCount)
	if epCount != 0 {
		t.Errorf("删除订阅后关联剧集应被级联清理，剩余 %d 条", epCount)
	}

	// 7. get_downloads 状态枚举校验与分页
	if resBadDL, _ := s.CallTool(ctx, "get_downloads", map[string]interface{}{"status": "invalid_state"}); !resBadDL.IsError {
		t.Errorf("get_downloads 非法状态筛选应报错")
	}
	if resComp, _ := s.CallTool(ctx, "get_downloads", map[string]interface{}{"status": "completed"}); resComp.IsError || !strings.Contains(resComp.Content[0].Text, "没有符合条件") {
		t.Errorf("get_downloads completed 筛选异常: %+v", resComp)
	}

	// 8. trigger_refresh 后台回调 panic 不应导致测试崩溃
	if resRef, _ := s.CallTool(ctx, "trigger_refresh", nil); resRef.IsError {
		t.Errorf("trigger_refresh 调用失败: %+v", resRef)
	}
	time.Sleep(30 * time.Millisecond)

	// 9. toggle_plugin 参数校验与真实切换
	if resBadTog, _ := s.CallTool(ctx, "toggle_plugin", map[string]interface{}{"plugin_id": "extended-notifiers", "enabled": "true"}); !resBadTog.IsError {
		t.Errorf("toggle_plugin 非布尔 enabled 应报错")
	}
	if resTog, _ := s.CallTool(ctx, "toggle_plugin", map[string]interface{}{"plugin_id": " extended-notifiers ", "enabled": false}); resTog.IsError {
		t.Errorf("toggle_plugin 停用插件失败: %+v", resTog)
	}

	// 10. get_system_status 版本格式化与下载器错误显示
	resSys, _ := s.CallTool(ctx, "get_system_status", nil)
	if resSys.IsError || strings.Contains(resSys.Content[0].Text, "vv0.6.0") || !strings.Contains(resSys.Content[0].Text, "v0.6.0") || !strings.Contains(resSys.Content[0].Text, "异常") {
		t.Errorf("get_system_status 输出异常: %+v", resSys)
	}

	// 11. CRITICAL: update_system_config 敏感键保护
	blockedKeys := []string{
		"jwt_secret",
		"JWT_SECRET",
		"jwt-secret",
		"token_secret",
		"MCP_TOKEN",
		"QB_PASS",
		"QB_PASSWORD",
		"TR_PASS",
		"ARIA2_SECRET",
		"TMDB_API_KEY",
		"AI_API_KEY",
		"BGMTV_USER_TOKEN",
		"WECHAT_APP_SECRET",
		"SERVERCHAN_KEY",
		"BARK_DEVICE_KEY",
		"plugin_status",
		"custom_plugins",
		"BAD KEY WITH SPACE",
		"BAD\nKEY",
	}
	for _, k := range blockedKeys {
		resBlock, _ := s.CallTool(ctx, "update_system_config", map[string]interface{}{
			"key":   k,
			"value": "pwned",
		})
		if !resBlock.IsError {
			t.Errorf("安全违规：敏感/非法配置键 %q 未被拦截！", k)
		}
		var cnt int64
		db.Model(&database.Setting{}).Where("key = ? AND value = ?", k, "pwned").Count(&cnt)
		if cnt != 0 {
			t.Errorf("安全违规：被拦截的配置键 %q 竟然写入了数据库！", k)
		}
	}

	if resBadVal, _ := s.CallTool(ctx, "update_system_config", map[string]interface{}{"key": "TV_BASE_PATH", "value": 123}); !resBadVal.IsError {
		t.Errorf("非字符串 value 应被拒绝")
	}
	if resOKCfg, _ := s.CallTool(ctx, "update_system_config", map[string]interface{}{"key": "TV_BASE_PATH", "value": "/media/Anime"}); resOKCfg.IsError {
		t.Errorf("合法配置项 TV_BASE_PATH 更新失败: %+v", resOKCfg)
	}
}
