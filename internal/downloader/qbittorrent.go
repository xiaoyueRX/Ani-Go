// Package downloader 实现各下载客户端的 Downloader 接口
// qBittorrent 客户端通过 Web API 与 qBittorrent 服务交互
package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
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
	loginMu    sync.Mutex
	loggedIn   bool
	lastLogin  time.Time
	retryCount int
}

// NewQBittorrent 创建 qBittorrent 下载器实例
func NewQBittorrent(host, username, password, category string) *QBittorrent {
	jar, _ := cookiejar.New(nil)
	client := httpx.New(30 * time.Second)
	client.Jar = jar
	return &QBittorrent{
		httpClient: client,
		host:     strings.TrimRight(host, "/"),
		username: username,
		password: password,
		category: category,
	}
}

func (q *QBittorrent) Name() string { return "qBittorrent" }

// login 登录 qBittorrent Web UI，获取会话 Cookie
func (q *QBittorrent) login(ctx context.Context) error {
	q.loginMu.Lock()
	defer q.loginMu.Unlock()

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
	req.Header.Set("Origin", q.host)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("登录请求失败: %w", err)
	}
	defer resp.Body.Close()

	// qBittorrent 登录成功返回 204 No Content，也可能是 200 OK
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("登录失败 (状态码 %d): %s", resp.StatusCode, string(body))
	}

	q.loggedIn = true
	q.lastLogin = time.Now()
	log.Println("✅ qBittorrent 登录成功")
	return nil
}

// Add 添加种子到 qBittorrent
func (q *QBittorrent) Add(ctx context.Context, item core.TorrentItem, savePath string) error {
	return q.addWithRetry(ctx, item, savePath, 0)
}

func (q *QBittorrent) addWithRetry(ctx context.Context, item core.TorrentItem, savePath string, attempt int) error {
	if attempt > 1 {
		return fmt.Errorf("添加种子重试次数超限: %s", item.Title)
	}

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
	// 构建标签: ani-go + 字幕组 + 分辨率
	tags := []string{"ani-go"}
	if item.GroupName != "" {
		tags = append(tags, item.GroupName)
	}
	if item.Resolution != "" {
		tags = append(tags, item.Resolution)
	}
	// qBittorrent API 使用逗号分隔标签（分号在 v4.3.4 之后也支持，但 v5.x 建议用逗号或数组）
	data.Set("tags", strings.Join(tags, ","))
	data.Set("autoTMM", "false")
	data.Set("paused", "false")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.host+"/api/v2/torrents/add", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("创建添加种子请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.host)
	req.Header.Set("Origin", q.host)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("添加种子请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("📥 添加种子响应: 状态码=%d, body=%s", resp.StatusCode, string(body))

	// qBittorrent API: 200 OK = 成功；201 Created = 成功；202 Accepted = 已接受（待处理）；204 No Content = 成功（无返回体）；403/401 = 未认证；409 = 已存在（幂等）；其他均视为需重试
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent {
		log.Printf("📥 已添加下载: %s", item.Title)
		return nil
	}

	// 409 = 已存在，也算成功（幂等）
	if resp.StatusCode == http.StatusConflict {
		log.Printf("📥 种子已存在，跳过: %s", item.Title)
		return nil
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		log.Printf("⚠️ 认证失败 (状态码 %d)，重新登录重试", resp.StatusCode)
	} else {
		log.Printf("⚠️ 添加种子返回异常状态码 %d，尝试重新登录重试", resp.StatusCode)
	}

	// 标记会话失效并重新登录
	q.loginMu.Lock()
	q.loggedIn = false
	q.loginMu.Unlock()
	if loginErr := q.login(ctx); loginErr != nil {
		return fmt.Errorf("重新登录失败: %w", loginErr)
	}

	// 重试添加
	log.Printf("🔄 重试添加种子 (%d/1): %s", attempt+1, item.Title)
	return q.addWithRetry(ctx, item, savePath, attempt+1)
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

	// 若遇到 401/403 认证过期，尝试重新登录并重试一次
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		q.loginMu.Lock()
		q.loggedIn = false
		q.loginMu.Unlock()
		if loginErr := q.login(ctx); loginErr != nil {
			return nil, fmt.Errorf("重新登录失败: %w", loginErr)
		}
		reqRetry, err := http.NewRequestWithContext(ctx, http.MethodGet,
			q.host+"/api/v2/torrents/info", nil)
		if err != nil {
			return nil, err
		}
		reqRetry.Header.Set("Referer", q.host)
		respRetry, err := q.httpClient.Do(reqRetry)
		if err != nil {
			return nil, fmt.Errorf("查询下载列表重试失败: %w", err)
		}
		defer respRetry.Body.Close()
		if respRetry.StatusCode == http.StatusNoContent {
			return []core.DownloadTask{}, nil
		}
		if respRetry.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("查询下载列表重试返回状态码: %d", respRetry.StatusCode)
		}
		return parseQBittorrentList(respRetry.Body)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("查询下载列表返回状态码: %d", resp.StatusCode)
	}

	// 204 = 空列表，返回空数组
	if resp.StatusCode == http.StatusNoContent {
		return []core.DownloadTask{}, nil
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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return core.DownloadTask{}, fmt.Errorf("查询种子状态返回状态码: %d", resp.StatusCode)
	}

	// 204 = 空结果，种子不存在
	if resp.StatusCode == http.StatusNoContent {
		return core.DownloadTask{}, fmt.Errorf("种子未找到: %s", hash)
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
	data.Set("deleteFiles", strconv.FormatBool(deleteFiles))

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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("删除种子失败 (状态码 %d): %s", resp.StatusCode, string(body))
	}

	log.Printf("🗑️  已删除下载任务: %s", hash)
	return nil
}

// Pause 暂停下载/做种任务
func (q *QBittorrent) Pause(ctx context.Context, hash string) error {
	if err := q.ensureLogin(ctx); err != nil {
		return err
	}

	data := url.Values{}
	data.Set("hashes", hash)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.host+"/api/v2/torrents/pause", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("创建暂停请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.host)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("暂停种子请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("暂停种子失败 (状态码 %d): %s", resp.StatusCode, string(body))
	}

	log.Printf("⏸️ [qBittorrent] 已暂停任务: %s", hash)
	return nil
}

// Resume 恢复下载/做种任务
func (q *QBittorrent) Resume(ctx context.Context, hash string) error {
	if err := q.ensureLogin(ctx); err != nil {
		return err
	}

	data := url.Values{}
	data.Set("hashes", hash)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.host+"/api/v2/torrents/resume", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("创建恢复请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.host)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("恢复种子请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("恢复种子失败 (状态码 %d): %s", resp.StatusCode, string(body))
	}

	log.Printf("▶️ [qBittorrent] 已恢复任务: %s", hash)
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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("添加标签失败 (状态码 %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// GetTorrentHashByURL 通过种子 URL 在下载列表中查找 hash
// qBittorrent Web API 不直接暴露种子来源 URL，因此通过 tag "ani-go" 过滤并返回最新添加的任务
func (q *QBittorrent) GetTorrentHashByURL(ctx context.Context, torrentURL string) (string, error) {
	tasks, err := q.List(ctx)
	if err != nil {
		return "", err
	}
	// 返回列表中最后一个带 hash 的任务（通常最新添加的即为刚投递的种子）
	for i := len(tasks) - 1; i >= 0; i-- {
		if tasks[i].Hash != "" {
			return tasks[i].Hash, nil
		}
	}
	return "", fmt.Errorf("未找到对应种子: %s", torrentURL)
}

// IsAvailable 检测 qBittorrent 是否可用
func (q *QBittorrent) IsAvailable(ctx context.Context) bool {
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet,
		q.host+"/api/v2/app/version", nil)
	if err != nil {
		return false
	}
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	// qBittorrent 可达返回 200，也可能返回 204
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent
}

// ensureLogin 确保已登录，如果未登录或 Cookie 过期则登录
func (q *QBittorrent) ensureLogin(ctx context.Context) error {
	q.loginMu.Lock()
	valid := q.loggedIn && time.Since(q.lastLogin) < 30*time.Minute
	q.loginMu.Unlock()
	if valid {
		return nil
	}
	return q.login(ctx)
}

// ============================================================
// qBittorrent JSON 响应解析（使用 encoding/json 标准库）
// ============================================================

// qbTorrentInfo qBittorrent API 返回的单个种子信息
type qbTorrentInfo struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	SavePath    string  `json:"save_path"`
	ContentPath string  `json:"content_path"`
	State       string  `json:"state"`
	Progress    float32 `json:"progress"`
	DlSpeed     int64   `json:"dlspeed"`
	UpSpeed     int64   `json:"upspeed"`
	Size        int64   `json:"size"`
	Completed   int64   `json:"completed"`
	Uploaded    int64   `json:"uploaded"`
	Ratio       float64 `json:"ratio"`
	SeedingTime int64   `json:"seeding_time"`
}

func parseQBittorrentList(r io.Reader) ([]core.DownloadTask, error) {
	var infos []qbTorrentInfo
	if err := json.NewDecoder(r).Decode(&infos); err != nil {
		if errors.Is(err, io.EOF) {
			return []core.DownloadTask{}, nil
		}
		return nil, fmt.Errorf("JSON 解码失败: %w", err)
	}

	tasks := make([]core.DownloadTask, 0, len(infos))
	for _, info := range infos {
		tasks = append(tasks, core.DownloadTask{
			Hash:         info.Hash,
			Name:         info.Name,
			SavePath:     info.SavePath,
			ContentPath:  info.ContentPath,
			Status:       info.State,
			Progress:     info.Progress,
			SpeedDown:    info.DlSpeed,
			SpeedUp:      info.UpSpeed,
			Size:         info.Size,
			Done:         info.Completed,
			Uploaded:     info.Uploaded,
			Ratio:        info.Ratio,
			SeedingTime:  info.SeedingTime,
		})
	}
	return tasks, nil
}
