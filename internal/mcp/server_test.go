package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
