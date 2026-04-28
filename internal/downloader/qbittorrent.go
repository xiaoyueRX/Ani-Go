// Package downloader 实现各下载客户端的 Downloader 接口
// qBittorrent 客户端通过 Web API 与 qBittorrent 服务交互
package downloader

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// ============================================================
// QBittorrent 客户端
// ============================================================

type QBittorrent struct {
	httpClient *http.Client
	host       string
	username   string
	password   string
	category   string
}

// NewQBittorrent 创建 qBittorrent 下载器实例
func NewQBittorrent(host, username, password, category string) *QBittorrent {
	jar, _ := cookiejar.New(nil)
	return &QBittorrent{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
		host:     strings.TrimRight(host, "/"),
		username: username,
		password: password,
		category: category,
	}
}

func (q *QBittorrent) Name() string { return "qBittorrent" }

// login 登录 qBittorrent Web UI，获取会话 Cookie
func (q *QBittorrent) login(ctx context.Context) error {
	data := url.Values{}
	data.Set("username", q.username)
	data.Set("password", q.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.host+"/api/v2/auth/login", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("创建登录请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.host)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("登录请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("登录失败 (状态码 %d): %s", resp.StatusCode, string(body))
	}

	log.Println("✅ qBittorrent 登录成功")
	return nil
}

// Add 添加种子到 qBittorrent
func (q *QBittorrent) Add(ctx context.Context, item core.TorrentItem, savePath string) error {
	if err := q.ensureLogin(ctx); err != nil {
		return err
	}

	data := url.Values{}
	// 优先使用磁力链接，否则使用 torrent URL
	if item.MagnetURL != "" {
		data.Set("urls", item.MagnetURL)
	} else {
		data.Set("urls", item.URL)
	}
	data.Set("savepath", savePath)
	data.Set("category", q.category)
	data.Set("autoTMM", "false")
	data.Set("paused", "false")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.host+"/api/v2/torrents/add", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("创建添加种子请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.host)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("添加种子请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("添加种子失败 (状态码 %d): %s", resp.StatusCode, string(body))
	}

	log.Printf("📥 已添加下载: %s", item.Title)
	return nil
}

// List 获取所有下载任务列表
func (q *QBittorrent) List(ctx context.Context) ([]core.DownloadTask, error) {
	if err := q.ensureLogin(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		q.host+"/api/v2/torrents/info", nil)
	if err != nil {
		return nil, fmt.Errorf("创建查询请求失败: %w", err)
	}
	req.Header.Set("Referer", q.host)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("查询下载列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("查询下载列表返回状态码: %d", resp.StatusCode)
	}

	// qBittorrent API 返回 JSON，这里用简单 JSON 解析
	tasks, err := parseQBittorrentList(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析下载列表失败: %w", err)
	}

	return tasks, nil
}

// GetStatus 获取指定种子状态
func (q *QBittorrent) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	if err := q.ensureLogin(ctx); err != nil {
		return core.DownloadTask{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		q.host+"/api/v2/torrents/info?hashes="+hash, nil)
	if err != nil {
		return core.DownloadTask{}, fmt.Errorf("创建查询请求失败: %w", err)
	}
	req.Header.Set("Referer", q.host)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return core.DownloadTask{}, fmt.Errorf("查询种子状态失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return core.DownloadTask{}, fmt.Errorf("查询种子状态返回状态码: %d", resp.StatusCode)
	}

	tasks, err := parseQBittorrentList(resp.Body)
	if err != nil {
		return core.DownloadTask{}, fmt.Errorf("解析种子状态失败: %w", err)
	}
	if len(tasks) == 0 {
		return core.DownloadTask{}, fmt.Errorf("种子未找到: %s", hash)
	}

	return tasks[0], nil
}

// Delete 删除下载任务
func (q *QBittorrent) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	if err := q.ensureLogin(ctx); err != nil {
		return err
	}

	data := url.Values{}
	data.Set("hashes", hash)
	if deleteFiles {
		data.Set("deleteFiles", "true")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.host+"/api/v2/torrents/delete", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("创建删除请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.host)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("删除种子请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("删除种子失败 (状态码 %d): %s", resp.StatusCode, string(body))
	}

	log.Printf("🗑️  已删除下载任务: %s", hash)
	return nil
}

// IsAvailable 检测 qBittorrent 是否可用
func (q *QBittorrent) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		q.host+"/api/v2/app/version", nil)
	if err != nil {
		return false
	}
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ensureLogin 确保已登录，如果 Cookie 过期则重新登录
func (q *QBittorrent) ensureLogin(ctx context.Context) error {
	if q.IsAvailable(ctx) {
		// 尝试访问需要认证的接口，如果失败则重新登录
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
			q.host+"/api/v2/torrents/info", nil)
		req.Header.Set("Referer", q.host)
		resp, err := q.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
	}
	return q.login(ctx)
}

// ============================================================
// qBittorrent JSON 响应解析
// ============================================================

func parseQBittorrentList(r io.Reader) ([]core.DownloadTask, error) {
	// 使用轻量 JSON 解析，避免引入第三方依赖
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// 简单手写 JSON 数组解析（qBittorrent 返回结构固定）
	// 格式: [{...},{...}]
	// 核心字段: hash, name, save_path, state, progress, dlspeed, size, completed
	rawJSON := strings.TrimSpace(string(body))
	if rawJSON == "[]" || rawJSON == "" {
		return nil, nil
	}

	return parseTorrentJSONArray(rawJSON), nil
}

// parseTorrentJSONArray 从 qBittorrent JSON 数组提取 DownloadTask 列表
// 不使用 encoding/json 以保持更小内存占用和更快的解析速度
func parseTorrentJSONArray(raw string) []core.DownloadTask {
	var tasks []core.DownloadTask

	// 按对象边界分割
	objects := splitJSONObjects(raw)
	for _, obj := range objects {
		task := core.DownloadTask{
			Hash:     extractJSONField(obj, "hash"),
			Name:     extractJSONField(obj, "name"),
			SavePath: extractJSONField(obj, "save_path"),
			Status:   extractJSONField(obj, "state"),
			Progress: parseFloatField(obj, "progress"),
			SpeedDown: parseIntField(obj, "dlspeed"),
			Size:     parseIntField(obj, "size"),
			Done:     parseIntField(obj, "completed"),
		}
		tasks = append(tasks, task)
	}

	return tasks
}

// splitJSONObjects 从 JSON 数组中提取每个对象字符串
func splitJSONObjects(raw string) []string {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '[' {
		return nil
	}
	raw = raw[1 : len(raw)-1] // 去掉外层 []

	var objects []string
	var depth, start int
	inString := false

	for i, ch := range raw {
		if ch == '"' && (i == 0 || raw[i-1] != '\\') {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 {
				objects = append(objects, raw[start:i+1])
			}
		}
	}

	return objects
}

// extractJSONField 从 JSON 对象中提取指定字段的字符串值
func extractJSONField(obj, field string) string {
	// 匹配 "field":"value" 或 "field": "value"
	pattern := fmt.Sprintf(`"%s"\s*:\s*"`, field)
	idx := strings.Index(obj, pattern)
	if idx == -1 {
		return ""
	}
	idx += len(pattern)
	end := idx
	for end < len(obj) {
		if obj[end] == '"' && (end == 0 || obj[end-1] != '\\') {
			break
		}
		end++
	}
	if end <= idx {
		return ""
	}
	return obj[idx:end]
}

// parseFloatField 从 JSON 对象中提取浮点字段
func parseFloatField(obj, field string) float32 {
	s := extractJSONField(obj, field)
	if s == "" {
		// 尝试非引号数值
		s = extractJSONFieldNum(obj, field)
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return float32(f)
}

// parseIntField 从 JSON 对象中提取整数字段
func parseIntField(obj, field string) int64 {
	s := extractJSONField(obj, field)
	if s == "" {
		s = extractJSONFieldNum(obj, field)
	}
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

// extractJSONFieldNum 从 JSON 对象中提取非引号的数值字段
func extractJSONFieldNum(obj, field string) string {
	pattern := fmt.Sprintf(`"%s"\s*:\s*`, field)
	idx := strings.Index(obj, pattern)
	if idx == -1 {
		return "0"
	}
	idx += len(pattern)

	// 跳过空格
	for idx < len(obj) && obj[idx] == ' ' {
		idx++
	}

	end := idx
	for end < len(obj) {
		ch := obj[end]
		if ch == ',' || ch == '}' || ch == ' ' || ch == '\n' || ch == '\r' {
			break
		}
		end++
	}

	if end == idx {
		return "0"
	}
	return obj[idx:end]
}
