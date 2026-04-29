package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// ============================================================
// Aria2 JSON-RPC 2.0 客户端
// 对接 aria2c --enable-rpc
// ============================================================

type Aria2 struct {
	httpClient *http.Client
	host       string
	token      string // RPC secret token
	reqID      atomic.Int32
}

type aria2Req struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int32         `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type aria2Resp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int32           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type aria2Status struct {
	GID             string `json:"gid"`
	Status          string `json:"status"`
	TotalLength     int64  `json:"totalLength,string"`
	CompletedLength int64  `json:"completedLength,string"`
	DownloadSpeed   int64  `json:"downloadSpeed,string"`
	Dir             string `json:"dir"`
	Bittorrent      *struct {
		InfoHash string `json:"infoHash"`
	} `json:"bittorrent"`
}

// NewAria2 创建 Aria2 下载器实例
func NewAria2(host, token string) *Aria2 {
	return &Aria2{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		host:       strings.TrimRight(host, "/"),
		token:      token,
	}
}

func (a *Aria2) Name() string { return "Aria2" }

// call 执行 JSON-RPC 调用
func (a *Aria2) call(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	// 如果设置了 token，在 params 前追加 "token:" 前缀
	// aria2 RPC 格式: method(params[0]=token:secret, params[1], ...)
	callParams := make([]interface{}, 0, len(params)+1)
	if a.token != "" {
		callParams = append(callParams, "token:"+a.token)
	}
	callParams = append(callParams, params...)

	id := a.reqID.Add(1)
	reqBody := aria2Req{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  callParams,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.host+"/jsonrpc", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建 Aria2 RPC 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Aria2 RPC 请求失败: %w", err)
	}
	defer resp.Body.Close()

	var arResp aria2Resp
	if err := json.NewDecoder(resp.Body).Decode(&arResp); err != nil {
		return nil, fmt.Errorf("Aria2 RPC 响应解析失败: %w", err)
	}
	if arResp.Error != nil {
		return nil, fmt.Errorf("Aria2 RPC 错误 [%d]: %s", arResp.Error.Code, arResp.Error.Message)
	}
	return arResp.Result, nil
}

// Add 添加下载任务
func (a *Aria2) Add(ctx context.Context, item core.TorrentItem, savePath string) error {
	uri := item.MagnetURL
	if uri == "" {
		uri = item.URL
	}
	params := []interface{}{[]string{uri}}
	if savePath != "" {
		params = append(params, map[string]string{"dir": savePath})
	}
	_, err := a.call(ctx, "aria2.addUri", params)
	if err != nil {
		return fmt.Errorf("Aria2 添加种子失败: %w", err)
	}
	log.Printf("📥 [Aria2] 已添加下载: %s", item.Title)
	return nil
}

// listAll 获取所有状态的任务
func (a *Aria2) listAll(ctx context.Context) ([]aria2Status, error) {
	var all []aria2Status

	for _, method := range []string{"aria2.tellActive", "aria2.tellWaiting", "aria2.tellStopped"} {
		var params []interface{}
		if method != "aria2.tellActive" {
			params = []interface{}{0, 1000}
		}
		raw, err := a.call(ctx, method, params)
		if err != nil {
			continue // 某个队列为空可能导致错误，跳过
		}
		var tasks []aria2Status
		if err := json.Unmarshal(raw, &tasks); err != nil {
			continue
		}
		all = append(all, tasks...)
	}
	return all, nil
}

// List 获取所有下载任务
func (a *Aria2) List(ctx context.Context) ([]core.DownloadTask, error) {
	tasks, err := a.listAll(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]core.DownloadTask, 0, len(tasks))
	for _, t := range tasks {
		hash := ""
		if t.Bittorrent != nil {
			hash = t.Bittorrent.InfoHash
		}
		result = append(result, core.DownloadTask{
			Hash:      hash,
			Name:      t.GID, // Aria2 没有 name 字段，用 GID
			SavePath:  t.Dir,
			Status:    t.Status,
			Progress:  float32(float64(t.CompletedLength) / float64(max(t.TotalLength, 1))),
			SpeedDown: t.DownloadSpeed,
			Size:      t.TotalLength,
			Done:      t.CompletedLength,
		})
	}
	return result, nil
}

// GetStatus 获取单个任务状态
func (a *Aria2) GetStatus(ctx context.Context, gid string) (core.DownloadTask, error) {
	raw, err := a.call(ctx, "aria2.tellStatus", []interface{}{gid})
	if err != nil {
		return core.DownloadTask{}, fmt.Errorf("Aria2 查询状态失败: %w", err)
	}
	var t aria2Status
	if err := json.Unmarshal(raw, &t); err != nil {
		return core.DownloadTask{}, fmt.Errorf("Aria2 解析状态失败: %w", err)
	}
	hash := ""
	if t.Bittorrent != nil {
		hash = t.Bittorrent.InfoHash
	}
	return core.DownloadTask{
		Hash:      hash,
		Name:      t.GID,
		SavePath:  t.Dir,
		Status:    t.Status,
		Progress:  float32(float64(t.CompletedLength) / float64(max(t.TotalLength, 1))),
		SpeedDown: t.DownloadSpeed,
		Size:      t.TotalLength,
		Done:      t.CompletedLength,
	}, nil
}

// Delete 删除下载任务
func (a *Aria2) Delete(ctx context.Context, gid string, deleteFiles bool) error {
	method := "aria2.remove"
	if deleteFiles {
		method = "aria2.removeDownloadResult"
	}
	_, err := a.call(ctx, method, []interface{}{gid})
	if err != nil {
		return fmt.Errorf("Aria2 删除任务失败: %w", err)
	}
	log.Printf("🗑️  [Aria2] 已删除下载任务: %s", gid)
	return nil
}

// IsAvailable 检测 Aria2 是否可用
func (a *Aria2) IsAvailable(ctx context.Context) bool {
	_, err := a.call(ctx, "aria2.getVersion", nil)
	return err == nil
}

// max 辅助函数
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
