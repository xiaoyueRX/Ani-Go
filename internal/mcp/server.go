package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ToolHandler func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error)

type toolEntry struct {
	tool    Tool
	handler ToolHandler
}

type sseSession struct {
	id      string
	msgChan chan string
	done    chan struct{}
}

// Server MCP 服务端核心管理器
type Server struct {
	mu          sync.RWMutex
	tokenGetter func() string
	tools       map[string]toolEntry
	sessions    map[string]*sseSession
}

// NewServer 创建新的 MCP 服务端
func NewServer(tokenGetter func() string) *Server {
	return &Server{
		tokenGetter: tokenGetter,
		tools:       make(map[string]toolEntry),
		sessions:    make(map[string]*sseSession),
	}
}

// RegisterTool 注册一个 MCP 工具及其实际处理函数
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = toolEntry{tool: tool, handler: handler}
	log.Printf("🔌 [MCP] 注册工具: %s", tool.Name)
}

// UnregisterTool 注销指定 MCP 工具（供插件停用时动态下架）
func (s *Server) UnregisterTool(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tools, name)
	log.Printf("🔌 [MCP] 注销工具: %s", name)
}

// ListTools 列出当前所有可用工具
func (s *Server) ListTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Tool, 0, len(s.tools))
	for _, entry := range s.tools {
		list = append(list, entry.tool)
	}
	return list
}

// CallTool 执行工具调用
func (s *Server) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolCallResult, error) {
	s.mu.RLock()
	entry, exists := s.tools[name]
	s.mu.RUnlock()

	if !exists {
		return ErrorResult(fmt.Sprintf("未找到名为 %q 的工具", name)), nil
	}

	// 异常恢复隔离罩
	var result *ToolCallResult
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("工具 %s 执行 panic: %v", name, r)
			}
		}()
		result, err = entry.handler(ctx, args)
	}()

	if err != nil {
		return ErrorResult(err.Error()), nil
	}
	return result, nil
}

// VerifyAuth 校验 Bearer Token
func (s *Server) VerifyAuth(r *http.Request) bool {
	if s.tokenGetter == nil {
		return false
	}
	configuredToken := strings.TrimSpace(s.tokenGetter())
	if configuredToken == "" {
		return false
	}

	// 1. Header 检查: Authorization: Bearer <token>
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			if strings.TrimSpace(parts[1]) == configuredToken {
				return true
			}
		}
	}

	// 2. Query 参数检查: ?token=<token>
	queryToken := r.URL.Query().Get("token")
	if queryToken != "" && queryToken == configuredToken {
		return true
	}

	return false
}

// HandleSSE 处理 GET /mcp/sse 请求
func (s *Server) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if !s.VerifyAuth(r) {
		http.Error(w, `{"error":"Unauthorized: 无效的 MCP Token"}`, http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	sessionID := generateSessionID()
	session := &sseSession{
		id:      sessionID,
		msgChan: make(chan string, 64),
		done:    make(chan struct{}),
	}

	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		close(session.done)
		log.Printf("🔌 [MCP] SSE 会话断开: %s", sessionID)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 发送初始 endpoint 事件 (符合 MCP SSE 规范)
	endpointURL := fmt.Sprintf("/mcp/messages?session_id=%s", sessionID)
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpointURL)
	flusher.Flush()
	log.Printf("🔌 [MCP] 新 SSE 会话建立: %s -> %s", sessionID, endpointURL)

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// 保持连接的心跳
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case msg, ok := <-session.msgChan:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// HandleMessages 处理 POST /mcp/messages 请求
func (s *Server) HandleMessages(w http.ResponseWriter, r *http.Request) {
	if !s.VerifyAuth(r) {
		http.Error(w, `{"error":"Unauthorized: 无效的 MCP Token"}`, http.StatusUnauthorized)
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "Missing session_id parameter", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	session, exists := s.sessions[sessionID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, fmt.Sprintf("Session %s not found or expired", sessionID), http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON-RPC request", http.StatusBadRequest)
		return
	}

	// 立即响应 202 Accepted 确认消息接收
	w.WriteHeader(http.StatusAccepted)

	// 后台处理并推送到 SSE 通道（使用独立上下文，避免 HTTP 连接关闭提前取消）
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		s.processJSONRPC(ctx, session, req)
	}()
}

func (s *Server) processJSONRPC(ctx context.Context, session *sseSession, req JSONRPCRequest) {
	var resp JSONRPCResponse
	resp.JSONRPC = "2.0"
	resp.ID = req.ID

	switch req.Method {
	case "initialize":
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]bool{
					"listChanged": true,
				},
			},
			"serverInfo": map[string]interface{}{
				"name":    "Ani-Go MCP Server",
				"version": "0.5.4",
			},
		}

	case "notifications/initialized":
		return // 客户端就绪确认通知，无需返回响应

	case "ping":
		resp.Result = map[string]interface{}{}

	case "tools/list":
		resp.Result = map[string]interface{}{
			"tools": s.ListTools(),
		}

	case "tools/call":
		var params ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Invalid params: %v", err),
			}
			break
		}

		result, err := s.CallTool(ctx, params.Name, params.Arguments)
		if err != nil {
			resp.Error = &JSONRPCError{
				Code:    -32603,
				Message: err.Error(),
			}
		} else {
			resp.Result = result
		}

	default:
		resp.Error = &JSONRPCError{
			Code:    -32601,
			Message: fmt.Sprintf("Method not found: %s", req.Method),
		}
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return
	}

	select {
	case session.msgChan <- string(data):
	case <-session.done:
	case <-ctx.Done():
	}
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
