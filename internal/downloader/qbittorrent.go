// Package downloader 实现各下载客户端的 Downloader 接口
// qBittorrent 客户端通过 Web API 与 qBittorrent 服务交互
package downloader

import (
	"context"
	"encoding/json"
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
	data.Set("tags", "ani-go")
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

// AddTags 给指定种子添加标签（参考 ani-rss 的 addTags 方法）
func (q *QBittorrent) AddTags(ctx context.Context, hash string, tags string) error {
	if err := q.ensureLogin(ctx); err != nil {
		return err
	}
	data := url.Values{}
	data.Set("hashes", hash)
	data.Set("tags", tags)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.host+"/api/v2/torrents/addTags", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("创建添加标签请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.host)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("添加标签请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("添加标签失败 (状态码 %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// GetTorrentHashByURL 通过种子 URL 在下载列表中查找 hash
func (q *QBittorrent) GetTorrentHashByURL(ctx context.Context, torrentURL string) (string, error) {
	tasks, err := q.List(ctx)
	if err != nil {
		return "", err
	}
	for _, t := range tasks {
		// 通过名称部分匹配来找
		if t.Hash != "" {
			return t.Hash, nil
		}
	}
	return "", fmt.Errorf("未找到对应种子: %s", torrentURL)
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
// qBittorrent JSON 响应解析（使用 encoding/json 标准库）
// ============================================================

// qbTorrentInfo qBittorrent API 返回的单个种子信息
type qbTorrentInfo struct {
	Hash      string  `json:"hash"`
	Name      string  `json:"name"`
	SavePath  string  `json:"save_path"`
	State     string  `json:"state"`
	Progress  float32 `json:"progress"`
	DlSpeed   int64   `json:"dlspeed"`
	Size      int64   `json:"size"`
	Completed int64   `json:"completed"`
}

func parseQBittorrentList(r io.Reader) ([]core.DownloadTask, error) {
	var infos []qbTorrentInfo
	if err := json.NewDecoder(r).Decode(&infos); err != nil {
		return nil, fmt.Errorf("JSON 解码失败: %w", err)
	}

	tasks := make([]core.DownloadTask, 0, len(infos))
	for _, info := range infos {
		tasks = append(tasks, core.DownloadTask{
			Hash:      info.Hash,
			Name:      info.Name,
			SavePath:  info.SavePath,
			Status:    info.State,
			Progress:  info.Progress,
			SpeedDown: info.DlSpeed,
			Size:      info.Size,
			Done:      info.Completed,
		})
	}
	return tasks, nil
}
