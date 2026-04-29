package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// ============================================================
// Transmission RPC 客户端
// 对接 Transmission JSON-RPC 协议
// ============================================================

type Transmission struct {
	httpClient *http.Client
	host       string
	username   string
	password   string

	mu        sync.Mutex
	sessionID string
}

type trRequest struct {
	Method    string                 `json:"method"`
	Arguments map[string]interface{} `json:"arguments"`
	Tag       int                    `json:"tag"`
}

type trResponse struct {
	Result    string          `json:"result"`
	Arguments json.RawMessage `json:"arguments"`
	Tag       int             `json:"tag"`
}

type trTorrent struct {
	ID              int     `json:"id"`
	HashString      string  `json:"hashString"`
	Name            string  `json:"name"`
	DownloadDir     string  `json:"downloadDir"`
	Status          int     `json:"status"`
	PercentDone     float64 `json:"percentDone"`
	RateDownload    int64   `json:"rateDownload"`
	TotalSize       int64   `json:"totalSize"`
	DownloadedEver  int64   `json:"downloadedEver"`
}

// 状态常量
const (
	trStatusStopped       = 0
	trStatusCheckWait     = 1
	trStatusCheck         = 2
	trStatusDownloadWait  = 3
	trStatusDownload      = 4
	trStatusSeedWait      = 5
	trStatusSeed          = 6
)

func trStatusString(status int) string {
	switch status {
	case trStatusStopped: return "stopped"
	case trStatusCheckWait, trStatusCheck: return "checking"
	case trStatusDownloadWait, trStatusDownload: return "downloading"
	case trStatusSeedWait, trStatusSeed: return "seeding"
	default: return "unknown"
	}
}

var trTagCounter int

// NewTransmission 创建 Transmission 下载器实例
func NewTransmission(host, username, password string) *Transmission {
	return &Transmission{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		host:       strings.TrimRight(host, "/"),
		username:   username,
		password:   password,
	}
}

func (t *Transmission) Name() string { return "Transmission" }

// call 执行 JSON-RPC 调用，自动处理 Session ID
func (t *Transmission) call(ctx context.Context, method string, args map[string]interface{}) (trResponse, error) {
	t.mu.Lock()
	sid := t.sessionID
	t.mu.Unlock()

	trTagCounter++
	tag := trTagCounter
	reqBody := trRequest{Method: method, Arguments: args, Tag: tag}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.host+"/transmission/rpc", bytes.NewReader(bodyBytes))
	if err != nil {
		return trResponse{}, fmt.Errorf("创建 Transmission RPC 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if sid != "" {
		req.Header.Set("X-Transmission-Session-Id", sid)
	}
	if t.username != "" {
		req.SetBasicAuth(t.username, t.password)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return trResponse{}, fmt.Errorf("Transmission RPC 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 409 Conflict → 服务端返回新 Session ID，需要重试
	if resp.StatusCode == http.StatusConflict {
		newSID := resp.Header.Get("X-Transmission-Session-Id")
		if newSID != "" {
			t.mu.Lock()
			t.sessionID = newSID
			t.mu.Unlock()
			return t.call(ctx, method, args)
		}
		return trResponse{}, fmt.Errorf("Transmission 返回 409 但未提供新 Session ID")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return trResponse{}, fmt.Errorf("Transmission RPC 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	var trResp trResponse
	if err := json.NewDecoder(resp.Body).Decode(&trResp); err != nil {
		return trResponse{}, fmt.Errorf("Transmission RPC 响应解析失败: %w", err)
	}
	if trResp.Result != "success" {
		return trResp, fmt.Errorf("Transmission RPC 返回错误: result=%s", trResp.Result)
	}
	return trResp, nil
}

// Add 添加种子（支持 magnet URL 和 torrent URL）
func (t *Transmission) Add(ctx context.Context, item core.TorrentItem, savePath string) error {
	args := map[string]interface{}{
		"paused": false,
	}
	if item.MagnetURL != "" {
		args["filename"] = item.MagnetURL
	} else {
		args["filename"] = item.URL
	}
	if savePath != "" {
		args["download-dir"] = savePath
	}

	_, err := t.call(ctx, "torrent-add", args)
	if err != nil {
		return fmt.Errorf("Transmission 添加种子失败: %w", err)
	}

	log.Printf("📥 [Transmission] 已添加下载: %s", item.Title)
	return nil
}

// List 获取所有下载任务
func (t *Transmission) List(ctx context.Context) ([]core.DownloadTask, error) {
	args := map[string]interface{}{
		"fields": []string{"id", "hashString", "name", "downloadDir", "status", "percentDone", "rateDownload", "totalSize", "downloadedEver"},
	}
	resp, err := t.call(ctx, "torrent-get", args)
	if err != nil {
		return nil, fmt.Errorf("Transmission 获取种子列表失败: %w", err)
	}

	var result struct {
		Torrents []trTorrent `json:"torrents"`
	}
	if err := json.Unmarshal(resp.Arguments, &result); err != nil {
		return nil, fmt.Errorf("Transmission 解析种子列表失败: %w", err)
	}

	tasks := make([]core.DownloadTask, 0, len(result.Torrents))
	for _, tr := range result.Torrents {
		tasks = append(tasks, core.DownloadTask{
			Hash:      tr.HashString,
			Name:      tr.Name,
			SavePath:  tr.DownloadDir,
			Status:    trStatusString(tr.Status),
			Progress:  float32(tr.PercentDone),
			SpeedDown: tr.RateDownload,
			Size:      tr.TotalSize,
			Done:      tr.DownloadedEver,
		})
	}
	return tasks, nil
}

// GetStatus 获取单个种子状态
func (t *Transmission) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	tasks, err := t.List(ctx)
	if err != nil {
		return core.DownloadTask{}, err
	}
	for _, task := range tasks {
		if task.Hash == hash {
			return task, nil
		}
	}
	return core.DownloadTask{}, fmt.Errorf("Transmission 种子未找到: %s", hash)
}

// Delete 删除下载任务
func (t *Transmission) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	args := map[string]interface{}{
		"ids":               []string{hash},
		"delete-local-data": deleteFiles,
	}
	_, err := t.call(ctx, "torrent-remove", args)
	if err != nil {
		return fmt.Errorf("Transmission 删除种子失败: %w", err)
	}
	log.Printf("🗑️  [Transmission] 已删除下载任务: %s", hash)
	return nil
}

// IsAvailable 检测 Transmission 是否可用
func (t *Transmission) IsAvailable(ctx context.Context) bool {
	args := map[string]interface{}{}
	_, err := t.call(ctx, "session-get", args)
	return err == nil
}
