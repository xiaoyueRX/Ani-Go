package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMCP_HighConcurrencyToolCalls 测试并发高负载下的工具调用与动态注册/注销
func TestMCP_HighConcurrencyToolCalls(t *testing.T) {
	s := NewServer(func() string { return "mcp-stress-token" })

	// 注册测试基础工具
	s.RegisterTool(Tool{
		Name:        "add_calc",
		Description: "加法计算器",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"a": {Type: "number"},
				"b": {Type: "number"},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		a, _ := args["a"].(float64)
		b, _ := args["b"].(float64)
		return TextResult(fmt.Sprintf("%g", a+b)), nil
	})

	// 注册一个可能 panic 的工具
	s.RegisterTool(Tool{
		Name:        "flaky_tool",
		Description: "偶发 panic 工具",
		InputSchema: InputSchema{Type: "object"},
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		shouldPanic, _ := args["panic"].(bool)
		if shouldPanic {
			panic("simulated fatal crash in flaky_tool")
		}
		return TextResult("ok"), nil
	})

	const numWorkers = 40
	const numOps = 25

	var wg sync.WaitGroup
	wg.Add(numWorkers + 2)

	// 1. 动态注册注销工具 goroutines
	stopReg := make(chan struct{})
	go func() {
		defer wg.Done()
		idx := 0
		for {
			select {
			case <-stopReg:
				return
			default:
				toolName := fmt.Sprintf("dynamic_tool_%d", idx%5)
				s.RegisterTool(Tool{Name: toolName}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
					return TextResult("dynamic"), nil
				})
				idx++
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()
	go func() {
		defer wg.Done()
		idx := 0
		for {
			select {
			case <-stopReg:
				return
			default:
				toolName := fmt.Sprintf("dynamic_tool_%d", idx%5)
				s.UnregisterTool(toolName)
				idx++
				time.Sleep(3 * time.Millisecond)
			}
		}
	}()

	// 2. 并发工作协程执行工具调用与列表获取
	for w := 0; w < numWorkers; w++ {
		workerID := w
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for i := 0; i < numOps; i++ {
				// ListTools
				_ = s.ListTools()

				// CallTool 正常计算
				res1, err1 := s.CallTool(ctx, "add_calc", map[string]interface{}{
					"a": float64(workerID),
					"b": float64(i),
				})
				if err1 != nil || res1.IsError {
					t.Errorf("add_calc 应该成功: %v", err1)
				}

				// CallTool panic 恢复隔离
				res2, err2 := s.CallTool(ctx, "flaky_tool", map[string]interface{}{
					"panic": (i % 2 == 0),
				})
				if err2 != nil {
					t.Errorf("CallTool 异常应被包装进 ToolCallResult 而不是返回 Go 错误: %v", err2)
				}
				if i%2 == 0 && !res2.IsError {
					t.Errorf("panic 工具应该返回 IsError=true 错误结果")
				}

				// 尝试调用动态工具（可能存在或已注销）
				_, _ = s.CallTool(ctx, fmt.Sprintf("dynamic_tool_%d", i%5), nil)
			}
		}()
	}

	time.Sleep(50 * time.Millisecond)
	close(stopReg)
	wg.Wait()
}

// TestMCP_MalformedJSONRPCRequests 测试极端畸形请求与非法 JSON 报文健壮性
func TestMCP_MalformedJSONRPCRequests(t *testing.T) {
	s := NewServer(func() string { return "test-token" })

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp/sse", s.HandleSSE)
	mux.HandleFunc("/mcp/messages", s.HandleMessages)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 先建立一个 SSE 会话获取有效 sessionID
	reqSSE, _ := http.NewRequest(http.MethodGet, ts.URL+"/mcp/sse", nil)
	reqSSE.Header.Set("Authorization", "Bearer test-token")

	// 模拟非法请求报文集合
	malformedBodies := []struct {
		name       string
		body       string
		sessionID  string
		authHeader string
		expectCode int
	}{
		{
			name:       "Missing Auth",
			body:       `{"jsonrpc":"2.0","method":"ping","id":1}`,
			sessionID:  "dummy-id",
			authHeader: "",
			expectCode: http.StatusUnauthorized,
		},
		{
			name:       "Missing Session ID",
			body:       `{"jsonrpc":"2.0","method":"ping","id":1}`,
			sessionID:  "",
			authHeader: "Bearer test-token",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Non-existent Session ID",
			body:       `{"jsonrpc":"2.0","method":"ping","id":1}`,
			sessionID:  "non-existent-session-12345",
			authHeader: "Bearer test-token",
			expectCode: http.StatusNotFound,
		},
	}

	for _, tc := range malformedBodies {
		t.Run(tc.name, func(t *testing.T) {
			url := ts.URL + "/mcp/messages"
			if tc.sessionID != "" {
				url += "?session_id=" + tc.sessionID
			}
			req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("创建测试请求失败: %v", err)
			}
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("发送测试请求失败: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectCode {
				t.Errorf("[%s] 状态码期望 %d, 得到 %d", tc.name, tc.expectCode, resp.StatusCode)
			}
		})
	}
}

// TestMCP_ContextCancellationDuringToolExecution 测试工具执行超时与上下文取消
func TestMCP_ContextCancellationDuringToolExecution(t *testing.T) {
	s := NewServer(func() string { return "token" })

	s.RegisterTool(Tool{
		Name: "slow_tool",
	}, func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error) {
		select {
		case <-time.After(1 * time.Second):
			return TextResult("finished"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	res, err := s.CallTool(ctx, "slow_tool", nil)
	if err != nil {
		t.Fatalf("CallTool 应捕获 context 错误: %v", err)
	}
	if !res.IsError {
		t.Errorf("超时被取消的工具执行结果应标记为 IsError")
	}
}
