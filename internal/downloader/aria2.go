package downloader

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
	"github.com/xiaoyueRX/Ani-Go/internal/magnet"
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
	FollowedBy      []string `json:"followedBy"`
	GID             string   `json:"gid"`
	InfoHash        string   `json:"infoHash"`
	Status          string   `json:"status"`
	TotalLength     int64    `json:"totalLength,string"`
	CompletedLength int64    `json:"completedLength,string"`
	DownloadSpeed   int64    `json:"downloadSpeed,string"`
	UploadSpeed     int64    `json:"uploadSpeed,string"`
	UploadLength    int64    `json:"uploadLength,string"`
	Dir             string   `json:"dir"`
	Files           []struct {
		Path string `json:"path"`
	} `json:"files"`
	Bittorrent *struct {
		InfoHash string `json:"infoHash"`
		Info     *struct {
			Name string `json:"name"`
		} `json:"info"`
	} `json:"bittorrent"`
}

// NewAria2 创建 Aria2 下载器实例
func NewAria2(host, token string) *Aria2 {
	h := strings.TrimRight(host, "/")
	if strings.HasSuffix(h, "/jsonrpc") {
		h = strings.TrimSuffix(h, "/jsonrpc")
	}
	return &Aria2{
		httpClient: httpx.New(30 * time.Second),
		host:       h,
		token:      token,
	}
}

func (a *Aria2) Name() string { return "Aria2" }

// call 执行 JSON-RPC 调用
func (a *Aria2) call(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	return a.callWithRetry(ctx, method, params, 0)
}

func (a *Aria2) callWithRetry(ctx context.Context, method string, params []interface{}, attempt int) (json.RawMessage, error) {
	if attempt > 3 {
		return nil, fmt.Errorf("Aria2 RPC 重试次数超限")
	}

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
		if method != "aria2.addUri" && ctx.Err() == nil && attempt < 2 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(50*(attempt+1)) * time.Millisecond):
				return a.callWithRetry(ctx, method, params, attempt+1)
			}
		}
		return nil, fmt.Errorf("Aria2 RPC 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if method != "aria2.addUri" && ctx.Err() == nil && attempt < 2 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(100*(attempt+1)) * time.Millisecond):
				return a.callWithRetry(ctx, method, params, attempt+1)
			}
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("Aria2 RPC 返回 HTTP 状态码 %d: %s", resp.StatusCode, string(body))
	}

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
	_, err := a.AddWithID(ctx, item, savePath)
	return err
}

func (a *Aria2) AddWithID(ctx context.Context, item core.TorrentItem, savePath string) (string, error) {
	// 磁盘空间熔断保护：在向下载器投递前执行熔断检查
	if err := CheckDiskSpaceGuard(savePath); err != nil {
		return "", err
	}

	uri := item.MagnetURL
	if uri == "" {
		uri = item.URL
	}
	if strings.TrimSpace(uri) == "" {
		return "", fmt.Errorf("Aria2 下载地址为空")
	}
	// 调度器为同一条持久投递提供稳定标签；普通调用仅在本次请求内共用 GID。
	nonce := item.TrackingTag
	if nonce == "" {
		nonce = rand.Text()
	}
	sum := sha256.Sum256([]byte("anigo-aria2-v1\x00" + nonce + "\x00" + uri + "\x00" + savePath))
	gid := fmt.Sprintf("%x", sum[:8])
	if gid == "0000000000000000" {
		gid = "0000000000000001"
	}
	options := map[string]string{"gid": gid}
	if savePath != "" {
		options["dir"] = savePath
	}
	raw, err := a.call(ctx, "aria2.addUri", []any{[]string{uri}, options})
	if err != nil {
		if ctx.Err() == nil && a.confirmSubmission(ctx, gid, uri, item.InfoHash) {
			return gid, nil
		}
		return "", fmt.Errorf("Aria2 未确认添加任务: %w", err)
	}
	var receipt string
	if err := json.Unmarshal(raw, &receipt); err != nil || !strings.EqualFold(receipt, gid) {
		return "", fmt.Errorf("Aria2 未返回所提交任务的 GID")
	}
	log.Printf("📥 [Aria2] 已添加下载: %s", item.Title)
	return gid, nil
}

func (a *Aria2) confirmSubmission(ctx context.Context, gid, uri, hash string) bool {
	raw, err := a.call(ctx, "aria2.tellStatus", []any{gid, []string{"gid", "status", "files", "infoHash", "followedBy"}})
	if err != nil {
		return false
	}
	var task struct {
		GID, Status, InfoHash string
		FollowedBy            []string
		Files                 []struct{ URIs []struct{ URI string } }
	}
	if err := json.Unmarshal(raw, &task); err != nil || !strings.EqualFold(task.GID, gid) {
		return false
	}
	switch task.Status {
	case "active", "waiting", "paused", "complete":
	default:
		return false
	}
	if h, err := magnet.Parse(uri); err == nil && h.V1 != "" {
		hash = h.V1
	}
	expected := magnet.NormalizeBTIH(hash)
	if expected != "" && strings.EqualFold(expected, task.InfoHash) {
		return true
	}
	for _, file := range task.Files {
		for _, u := range file.URIs {
			if u.URI == uri {
				return true
			}
		}
	}
	// 元数据任务完成后，实际种子可能只在子 GID 的状态中报告 infoHash。
	if expected != "" && len(task.FollowedBy) == 1 {
		child, err := a.call(ctx, "aria2.tellStatus", []any{task.FollowedBy[0], []string{"gid", "infoHash"}})
		if err == nil {
			var detail struct{ GID, InfoHash string }
			return json.Unmarshal(child, &detail) == nil && strings.EqualFold(detail.GID, task.FollowedBy[0]) && strings.EqualFold(detail.InfoHash, expected)
		}
	}
	return false
}

// listAll 获取所有状态的任务
func (a *Aria2) listAll(ctx context.Context) ([]aria2Status, error) {
	var all []aria2Status
	var lastErr error
	successCount := 0

	for _, method := range []string{"aria2.tellActive", "aria2.tellWaiting", "aria2.tellStopped"} {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var params []interface{}
		if method != "aria2.tellActive" {
			params = []interface{}{0, 1000}
		}
		raw, err := a.call(ctx, method, params)
		if err != nil {
			lastErr = err
			continue // 某个队列为空或异常，尝试其他队列
		}
		successCount++
		var tasks []aria2Status
		if err := json.Unmarshal(raw, &tasks); err == nil {
			all = append(all, tasks...)
		}
	}

	// 如果全部队列查询均失败，返回底层错误
	if successCount == 0 && lastErr != nil {
		return nil, fmt.Errorf("Aria2 查询任务列表全部失败: %w", lastErr)
	}

	return all, nil
}

func convertAria2Status(t aria2Status) core.DownloadTask {
	hash := t.InfoHash
	if hash == "" && t.Bittorrent != nil {
		hash = t.Bittorrent.InfoHash
	}
	if hash == "" {
		hash = t.GID
	}
	ratio := 0.0
	if t.CompletedLength > 0 {
		ratio = float64(t.UploadLength) / float64(t.CompletedLength)
	}

	// 状态规范化：将 aria2 的 "complete" 映射为 "completed"
	status := t.Status
	if status == "complete" {
		status = "completed"
	}

	// 任务名称解析：优先提取种子元数据名称，其次提取实际下载文件/文件夹名
	name := t.GID
	if t.Bittorrent != nil && t.Bittorrent.Info != nil && strings.TrimSpace(t.Bittorrent.Info.Name) != "" {
		name = strings.TrimSpace(t.Bittorrent.Info.Name)
	} else if len(t.Files) > 0 && t.Files[0].Path != "" {
		rel, err := filepath.Rel(t.Dir, t.Files[0].Path)
		if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			segs := strings.Split(filepath.ToSlash(rel), "/")
			if len(segs) > 0 && segs[0] != "" {
				name = segs[0]
			}
		} else {
			base := filepath.Base(t.Files[0].Path)
			if base != "." && base != "/" && base != "\\" && base != "" {
				name = base
			}
		}
	}

	var remaining *int64
	if t.TotalLength > 0 {
		n := t.TotalLength - t.CompletedLength
		if n < 0 {
			n = 0
		}
		remaining = &n
	}
	return core.DownloadTask{
		Hash:           hash,
		ClientID:       t.GID,
		FollowedBy:     t.FollowedBy,
		Name:           name,
		SavePath:       t.Dir,
		Status:         status,
		Progress:       downloadProgress(float64(t.CompletedLength)/float64(max(t.TotalLength, 1)), remaining),
		RemainingBytes: remaining,
		SpeedDown:      t.DownloadSpeed,
		SpeedUp:        t.UploadSpeed,
		Size:           t.TotalLength,
		Done:           t.CompletedLength,
		Uploaded:       t.UploadLength,
		Ratio:          ratio,
		SeedingTime:    0,
	}
}

// List 获取所有下载任务
func (a *Aria2) List(ctx context.Context) ([]core.DownloadTask, error) {
	tasks, err := a.listAll(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]core.DownloadTask, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, convertAria2Status(t))
	}
	return result, nil
}

// resolveGID 将传入的 GID 或 Torrent Hash 转换为可操作的 GID
func (a *Aria2) resolveGID(ctx context.Context, hashOrGID string) string {
	target := strings.TrimSpace(hashOrGID)
	if target == "" {
		return ""
	}
	if len(target) == 16 {
		return target
	}
	tasks, err := a.listAll(ctx)
	if err == nil {
		for _, t := range tasks {
			if strings.EqualFold(t.InfoHash, target) || (t.Bittorrent != nil && strings.EqualFold(t.Bittorrent.InfoHash, target)) {
				return t.GID
			}
			if strings.EqualFold(t.GID, target) {
				return t.GID
			}
		}
	}
	return target
}

// GetStatus 获取单个任务状态（支持 GID 或 Torrent Hash）
func (a *Aria2) GetStatus(ctx context.Context, hashOrGID string) (core.DownloadTask, error) {
	target := strings.TrimSpace(hashOrGID)
	if target == "" {
		return core.DownloadTask{}, fmt.Errorf("Aria2 查询目标不能为空")
	}

	// 1. 尝试使用 target 作为 GID 直接查询
	raw, err := a.call(ctx, "aria2.tellStatus", []interface{}{target})
	if err == nil {
		var t aria2Status
		if err := json.Unmarshal(raw, &t); err == nil && t.GID != "" {
			return convertAria2Status(t), nil
		}
	}

	// 2. 如果直接查询失败，在任务列表中按 Hash 或 GID 匹配
	tasks, listErr := a.List(ctx)
	if listErr == nil {
		for _, task := range tasks {
			if strings.EqualFold(task.Hash, target) || strings.EqualFold(task.Name, target) {
				return task, nil
			}
		}
	}

	return core.DownloadTask{}, fmt.Errorf("Aria2 种子/任务未找到: %s", hashOrGID)
}

// Delete 删除下载任务（支持 GID 或 Torrent Hash）
func (a *Aria2) Delete(ctx context.Context, hashOrGID string, deleteFiles bool) error {
	target := strings.TrimSpace(hashOrGID)
	if target == "" {
		return fmt.Errorf("Aria2 删除目标不能为空")
	}

	gid := a.resolveGID(ctx, target)
	var filesToDelete []string
	if deleteFiles {
		if raw, err := a.call(ctx, "aria2.tellStatus", []interface{}{gid}); err == nil {
			var st aria2Status
			if err := json.Unmarshal(raw, &st); err == nil {
				for _, f := range st.Files {
					if f.Path != "" {
						filesToDelete = append(filesToDelete, f.Path)
					}
				}
			}
		}
	}

	// 先尝试 aria2.remove（针对 active/waiting/paused 任务）
	_, err := a.call(ctx, "aria2.remove", []interface{}{gid})
	if err != nil {
		// 若不是活动任务或移除失败，尝试 aria2.removeDownloadResult（针对 stopped/complete/error 任务）
		if _, err2 := a.call(ctx, "aria2.removeDownloadResult", []interface{}{gid}); err2 != nil {
			// 最后尝试 forceRemove
			if _, err3 := a.call(ctx, "aria2.forceRemove", []interface{}{gid}); err3 != nil {
				return fmt.Errorf("Aria2 删除任务失败: remove=%v, removeResult=%v", err, err2)
			}
		}
	}

	// 如果需要删除本地文件
	if deleteFiles {
		for _, p := range filesToDelete {
			_ = os.Remove(p)
			_ = os.Remove(p + ".aria2")
		}
	}

	log.Printf("🗑️  [Aria2] 已删除下载任务: %s (GID=%s, deleteFiles=%v)", target, gid, deleteFiles)
	return nil
}

// Pause 暂停下载任务（支持 GID 或 Torrent Hash）
func (a *Aria2) Pause(ctx context.Context, hashOrGID string) error {
	target := strings.TrimSpace(hashOrGID)
	if target == "" {
		return fmt.Errorf("Aria2 暂停目标不能为空")
	}

	gid := a.resolveGID(ctx, target)
	_, err := a.call(ctx, "aria2.pause", []interface{}{gid})
	if err != nil {
		return fmt.Errorf("Aria2 暂停任务失败: %w", err)
	}
	log.Printf("⏸️  [Aria2] 已暂停任务: %s (GID=%s)", target, gid)
	return nil
}

// Resume 恢复下载任务（支持 GID 或 Torrent Hash）
func (a *Aria2) Resume(ctx context.Context, hashOrGID string) error {
	target := strings.TrimSpace(hashOrGID)
	if target == "" {
		return fmt.Errorf("Aria2 恢复目标不能为空")
	}

	gid := a.resolveGID(ctx, target)
	_, err := a.call(ctx, "aria2.unpause", []interface{}{gid})
	if err != nil {
		return fmt.Errorf("Aria2 恢复任务失败: %w", err)
	}
	log.Printf("▶️  [Aria2] 已恢复任务: %s (GID=%s)", target, gid)
	return nil
}

// IsAvailable 检测 Aria2 是否可用（含 5s 超时保护）
func (a *Aria2) IsAvailable(ctx context.Context) bool {
	if strings.TrimSpace(a.host) == "" {
		return false
	}

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := a.call(checkCtx, "aria2.getVersion", nil)
	return err == nil
}

// max 辅助函数
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
