package mcp

import "encoding/json"

// JSONRPCRequest 标准 JSON-RPC 2.0 请求
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse 标准 JSON-RPC 2.0 响应
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError 标准 JSON-RPC 错误
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Tool MCP 协议工具定义
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema JSON Schema 参数描述
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property 单个参数属性描述
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

// ToolCallParams tools/call 请求参数
type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCallResult 工具调用结果
type ToolCallResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ToolContent 内容载体
type ToolContent struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// TextResult 快速构建纯文本工具响应
func TextResult(text string) *ToolCallResult {
	return &ToolCallResult{
		Content: []ToolContent{
			{Type: "text", Text: text},
		},
	}
}

// ErrorResult 快速构建错误工具响应
func ErrorResult(errMsg string) *ToolCallResult {
	return &ToolCallResult{
		Content: []ToolContent{
			{Type: "text", Text: errMsg},
		},
		IsError: true,
	}
}
