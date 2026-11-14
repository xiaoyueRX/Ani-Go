package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultSessionTimeout    = 30 * time.Minute
	defaultHeartbeatInterval = 20 * time.Second
	defaultMaxSessions       = 256
	defaultMaxBodyBytes      = 1 << 20 // 1 MiB 请求体上限，防止恶意大报文耗尽内存
	defaultSendTimeout       = 10 * time.Second
	defaultServerVersion     = "0.6.0"
)

// ToolHandler 工具执行回调函数签名
type ToolHandler func(ctx context.Context, args map[string]interface{}) (*ToolCallResult, error)

type toolEntry struct {
	tool    Tool
	handler ToolHandler
}

type sseSession struct {
	id         string
	msgChan    chan string
	done       chan struct{}
	closeOnce  sync.Once
	lastActive atomic.Int64 // 最近一次活跃的 UnixNano 时间戳
}

func (sess *sseSession) touch() {
	sess.lastActive.Store(time.Now().UnixNano())
}

func (sess *sseSession) lastActiveTime() time.Time {
	return time.Unix(0, sess.lastActive.Load())
}

func (sess *sseSession) close() {
	sess.closeOnce.Do(func() {
		close(sess.done)
	})
}

func (sess *sseSession) isClosed() bool {
	select {
	case <-sess.done:
		return true
	default:
		return false
	}
}

// Server MCP 服务端核心管理器
type Server struct {
	mu                sync.RWMutex
	tokenGetter       func() string
	tools             map[string]toolEntry
	sessions          map[string]*sseSession
	sessionTimeout    time.Duration
	heartbeatInterval time.Duration
	maxSessions       int
	maxBodyBytes      int64
	version           string
}

// NewServer 创建新的 MCP 服务端
func NewServer(tokenGetter func() string) *Server {
	return &Server{
		tokenGetter:       tokenGetter,
		tools:             make(map[string]toolEntry),
		sessions:          make(map[string]*sseSession),
		sessionTimeout:    defaultSessionTimeout,
		heartbeatInterval: defaultHeartbeatInterval,
		maxSessions:       defaultMaxSessions,
		maxBodyBytes:      defaultMaxBodyBytes,
		version:           defaultServerVersion,
	}
}

// RegisterTool 注册一个 MCP 工具及其实际处理函数
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	tool.Name = strings.TrimSpace(tool.Name)
	if tool.Name == "" || handler == nil {
		log.Printf("⚠️ [MCP] 忽略无效工具注册: name=%q, handlerNil=%v", tool.Name, handler == nil)
		return
	}
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

// HasTool 检查指定工具是否已注册
func (s *Server) HasTool(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.tools[name]
	return exists
}

// ListTools 按工具名排序列出当前所有可用工具
func (s *Server) ListTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Tool, 0, len(s.tools))
	for _, entry := range s.tools {
		list = append(list, entry.tool)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

// CallTool 执行工具调用（带 panic 恢复与空参数保护）
func (s *Server) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolCallResult, error) {
	s.mu.RLock()
	entry, exists := s.tools[name]
	s.mu.RUnlock()

	if !exists || entry.handler == nil {
		return ErrorResult(fmt.Sprintf("未找到名为 %q 的工具", name)), nil
	}

	if args == nil {
		args = map[string]interface{}{}
	}

	// 异常恢复隔离罩，防止单个工具崩溃影响主进程
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
	if result == nil {
		return TextResult(""), nil
	}
	return result, nil
}

// VerifyAuth 使用常量时间比较校验 Bearer Token
func (s *Server) VerifyAuth(r *http.Request) bool {
	if r == nil || s.tokenGetter == nil {
		return false
	}
	configuredToken := strings.TrimSpace(s.tokenGetter())
	if configuredToken == "" {
		return false
	}

	// 1. Header 检查: Authorization: Bearer <token>
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader != "" {
		fields := strings.Fields(authHeader)
		if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			if constantTimeTokenEqual(fields[1], configuredToken) {
				return true
			}
		}
	}

	// 2. Query 参数检查: ?token=<token>
	if r.URL != nil {
		queryToken := strings.TrimSpace(r.URL.Query().Get("token"))
		if queryToken != "" && constantTimeTokenEqual(queryToken, configuredToken) {
			return true
		}
	}

	return false
}

// constantTimeTokenEqual 使用 crypto/subtle.ConstantTimeCompare 进行恒定时间比对，避免时序侧信道攻击
func constantTimeTokenEqual(candidate, expected string) bool {
	if candidate == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(expected)) == 1
}

// EvictStaleSessions 主动清理所有超时未活跃的僵尸 SSE 会话并返回清理数量
func (s *Server) EvictStaleSessions() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evictStaleSessionsLocked(time.Now())
}

func (s *Server) evictStaleSessionsLocked(now time.Time) int {
	if s.sessionTimeout <= 0 {
		return 0
	}
	evicted := 0
	for id, sess := range s.sessions {
		if now.Sub(sess.lastActiveTime()) > s.sessionTimeout {
			sess.close()
			delete(s.sessions, id)
			evicted++
		}
	}
	return evicted
}

// SessionCount 返回当前活跃 SSE 会话数量
func (s *Server) SessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// Close 关闭并清理所有活跃 SSE 会话
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		sess.close()
		delete(s.sessions, id)
	}
}

// HandleSSE 处理 GET /mcp/sse 请求
func (s *Server) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.VerifyAuth(r) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"Unauthorized: 无效的 MCP Token"}`, http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	sessionID, err := generateSessionID()
	if err != nil {
		http.Error(w, "Failed to allocate session", http.StatusInternalServerError)
		return
	}

	session := &sseSession{
		id:      sessionID,
		msgChan: make(chan string, 64),
		done:    make(chan struct{}),
	}
	session.touch()

	// 注册会话前先淘汰过期会话，并限制最大并发会话数
	s.mu.Lock()
	s.evictStaleSessionsLocked(time.Now())
	if s.maxSessions > 0 && len(s.sessions) >= s.maxSessions {
		s.mu.Unlock()
		http.Error(w, "Too many active MCP sessions", http.StatusServiceUnavailable)
		return
	}
	s.sessions[sessionID] = session
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if current, exists := s.sessions[sessionID]; exists && current == session {
			delete(s.sessions, sessionID)
		}
		s.mu.Unlock()
		session.close()
		log.Printf("🔌 [MCP] SSE 会话断开: %s", sessionID)
	}()

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	// 发送初始 endpoint 事件 (符合 MCP SSE 规范)
	endpointURL := fmt.Sprintf("/mcp/messages?session_id=%s", sessionID)
	if _, err := fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpointURL); err != nil {
		return
	}
	flusher.Flush()
	log.Printf("🔌 [MCP] 新 SSE 会话建立: %s -> %s", sessionID, endpointURL)

	hbInterval := s.heartbeatInterval
	if hbInterval <= 0 {
		hbInterval = defaultHeartbeatInterval
	}
	ticker := time.NewTicker(hbInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-session.done:
			return
		case <-ticker.C:
			// 检测会话是否已超时未活跃，主动回收僵尸连接
			if s.sessionTimeout > 0 && time.Since(session.lastActiveTime()) > s.sessionTimeout {
				log.Printf("🔌 [MCP] SSE 会话空闲超时回收: %s", sessionID)
				return
			}
			// 保持连接的心跳，若底层写入失败则立即退出释放资源
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case msg, ok := <-session.msgChan:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// HandleMessages 处理 POST /mcp/messages 请求
func (s *Server) HandleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.VerifyAuth(r) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"Unauthorized: 无效的 MCP Token"}`, http.StatusUnauthorized)
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		http.Error(w, "Missing session_id parameter", http.StatusBadRequest)
		return
	}

	session, exists := s.lookupActiveSession(sessionID)
	if !exists {
		http.Error(w, fmt.Sprintf("Session %s not found or expired", sessionID), http.StatusNotFound)
		return
	}

	maxBody := s.maxBodyBytes
	if maxBody <= 0 {
		maxBody = defaultMaxBodyBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	req, ok := parseJSONRPCRequest(body)
	if !ok {
		http.Error(w, "Invalid JSON-RPC request", http.StatusBadRequest)
		return
	}

	if session.isClosed() {
		http.Error(w, fmt.Sprintf("Session %s not found or expired", sessionID), http.StatusNotFound)
		return
	}

	// 更新会话活跃时间戳
	session.touch()

	// 立即响应 202 Accepted 确认消息接收
	w.WriteHeader(http.StatusAccepted)

	// 后台处理并推送到 SSE 通道（使用独立上下文，避免 HTTP 连接关闭提前取消）
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("❌ [MCP] 处理 JSON-RPC 发生 panic: %v", rec)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		s.processJSONRPC(ctx, session, req)
	}()
}

// lookupActiveSession 查找会话并检查是否已过期；若已过期则顺带淘汰
func (s *Server) lookupActiveSession(sessionID string) (*sseSession, bool) {
	s.mu.RLock()
	session, exists := s.sessions[sessionID]
	s.mu.RUnlock()
	if !exists {
		return nil, false
	}

	if s.sessionTimeout > 0 && time.Since(session.lastActiveTime()) > s.sessionTimeout {
		s.mu.Lock()
		if current, ok := s.sessions[sessionID]; ok && current == session && time.Since(current.lastActiveTime()) > s.sessionTimeout {
			current.close()
			delete(s.sessions, sessionID)
		}
		s.mu.Unlock()
		return nil, false
	}

	if session.isClosed() {
		return nil, false
	}
	return session, true
}

// parseJSONRPCRequest 严格解析单个 JSON-RPC 对象，拒绝 null、数组及尾部多余数据
func parseJSONRPCRequest(body []byte) (JSONRPCRequest, bool) {
	var req JSONRPCRequest
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return req, false
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	if err := dec.Decode(&req); err != nil {
		return req, false
	}

	// 确保报文末尾没有额外夹带的 JSON 值
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		return req, false
	}
	return req, true
}

func (s *Server) processJSONRPC(ctx context.Context, session *sseSession, req JSONRPCRequest) {
	var resp JSONRPCResponse
	resp.JSONRPC = "2.0"
	resp.ID = req.ID

	method := strings.TrimSpace(req.Method)
	switch {
	case req.JSONRPC != "2.0":
		resp.Error = &JSONRPCError{
			Code:    -32600,
			Message: "Invalid Request: jsonrpc must be \"2.0\"",
		}

	case method == "":
		resp.Error = &JSONRPCError{
			Code:    -32600,
			Message: "Invalid Request: method is required",
		}

	case method == "notifications/initialized" || strings.HasPrefix(method, "notifications/"):
		// JSON-RPC 2.0 规范：通知类消息不返回任何响应
		return

	case method == "initialize":
		ver := strings.TrimPrefix(strings.TrimSpace(s.version), "v")
		if ver == "" {
			ver = defaultServerVersion
		}
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]bool{
					"listChanged": true,
				},
			},
			"serverInfo": map[string]interface{}{
				"name":    "Ani-Go MCP Server",
				"version": ver,
			},
		}

	case method == "ping":
		resp.Result = map[string]interface{}{}

	case method == "tools/list":
		resp.Result = map[string]interface{}{
			"tools": s.ListTools(),
		}

	case method == "tools/call":
		trimmedParams := bytes.TrimSpace(req.Params)
		if len(trimmedParams) == 0 || string(trimmedParams) == "null" {
			resp.Error = &JSONRPCError{
				Code:    -32602,
				Message: "Invalid params: params object is required",
			}
			break
		}

		var params ToolCallParams
		if err := json.Unmarshal(trimmedParams, &params); err != nil {
			resp.Error = &JSONRPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Invalid params: %v", err),
			}
			break
		}

		params.Name = strings.TrimSpace(params.Name)
		if params.Name == "" {
			resp.Error = &JSONRPCError{
				Code:    -32602,
				Message: "Invalid params: tool name is required",
			}
			break
		}

		if !s.HasTool(params.Name) {
			resp.Error = &JSONRPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Unknown tool: %s", params.Name),
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
			Message: fmt.Sprintf("Method not found: %s", method),
		}
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return
	}

	sendTimer := time.NewTimer(defaultSendTimeout)
	defer sendTimer.Stop()

	select {
	case session.msgChan <- string(data):
	case <-session.done:
	case <-ctx.Done():
	case <-sendTimer.C:
		log.Printf("⚠️ [MCP] 会话 %s SSE 发送队列已满，丢弃响应 (method=%s)", session.id, method)
	}
}

func generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
