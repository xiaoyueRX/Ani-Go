// Package cloud115 向 115 开放平台提交磁力链接并查询云任务列表。
package cloud115

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	magnetlink "github.com/xiaoyueRX/Ani-Go/internal/magnet"
)

const baseURL = "https://proapi.115.com/open/offline/"

var (
	ErrInvalidMagnet = magnetlink.ErrInvalid
	ErrUnauthorized  = errors.New("115 访问令牌无效或授权已过期")
	ErrRateLimited   = errors.New("115 请求过于频繁，请稍后再试")
	errInvalidReply  = errors.New("115 返回了无效的响应")
)

type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{http: &http.Client{
		Timeout:       15 * time.Second,
		Transport:     http.DefaultTransport.(*http.Transport).Clone(),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (c *Client) CloseIdleConnections() { c.http.CloseIdleConnections() }

type TaskPage struct {
	Page      int    `json:"page"`
	PageCount int    `json:"page_count"`
	Tasks     []Task `json:"tasks"`
}

type Task struct {
	InfoHash string `json:"info_hash"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
}

// ValidateMagnet 校验磁力链接的 xt 参数，支持 BitTorrent v1 的十六进制或 Base32 哈希以及 v2 的 SHA-256 哈希。
func ValidateMagnet(link string) error {
	_, err := magnetlink.Parse(link)
	return err
}

// 115 使用 SHA-1 作为任务 ID。对于仅含 v2 哈希的请求，
// 需核对回执中磁力链接的 SHA-256，以确认提交成功的是同一任务。
func matchesReceipt(want magnetlink.Hashes, infoHash, echoedURL string) bool {
	received := magnetlink.NormalizeBTIH(infoHash)
	if received == "" || (want.V1 != "" && want.V1 != received) {
		return false
	}
	if echoedURL == "" {
		return want.V1 != ""
	}
	echoed, err := magnetlink.Parse(echoedURL)
	if err != nil || (echoed.V1 != "" && echoed.V1 != received) ||
		(want.V2 != "" && echoed.V2 != "" && echoed.V2 != want.V2) {
		return false
	}
	return (want.V1 != "" && want.V1 == echoed.V1) || (want.V2 != "" && want.V2 == echoed.V2)
}

// Add 通过 115 表单的 urls 字段提交一条链接，收到与该链接匹配的回执才算成功。
func (c *Client) Add(ctx context.Context, accessToken, link string) error {
	expected, err := magnetlink.Parse(link)
	if err != nil {
		return err
	}
	body := url.Values{"urls": {link}}.Encode()
	raw, err := c.request(ctx, accessToken, http.MethodPost, "add_task_urls", strings.NewReader(body))
	if err != nil {
		return err
	}
	var items []struct {
		result
		InfoHash string `json:"info_hash"`
		URL      string `json:"url"`
	}
	if err := json.Unmarshal(raw, &items); err != nil || len(items) != 1 {
		return errInvalidReply
	}
	if err := items[0].check(); err != nil {
		return err
	}
	if !matchesReceipt(expected, items[0].InfoHash, items[0].URL) {
		return errInvalidReply
	}
	return nil
}

// List 读取一页任务，只向调用方返回插件使用的字段。
func (c *Client) List(ctx context.Context, accessToken string, page int) (TaskPage, error) {
	if page < 1 {
		return TaskPage{}, errors.New("115 任务页码必须为正数")
	}
	accessToken = strings.TrimSpace(accessToken)
	raw, err := c.request(ctx, accessToken, http.MethodGet, "get_task_list?page="+strconv.Itoa(page), nil)
	if err != nil {
		return TaskPage{}, err
	}
	var data struct {
		Page      int               `json:"page"`
		PageCount *int              `json:"page_count"`
		Count     *int              `json:"count"`
		Tasks     []json.RawMessage `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.Page < 1 || data.PageCount == nil || *data.PageCount < 0 {
		return TaskPage{}, errInvalidReply
	}
	if data.Tasks == nil && (*data.PageCount > 0 || (data.Count != nil && *data.Count > 0)) {
		return TaskPage{}, errInvalidReply
	}
	p := TaskPage{Page: data.Page, PageCount: *data.PageCount, Tasks: make([]Task, 0, len(data.Tasks))}
	for _, row := range data.Tasks {
		var item struct {
			InfoHash string          `json:"info_hash"`
			Name     json.RawMessage `json:"name"`
			Status   json.RawMessage `json:"status"`
			Percent  json.RawMessage `json:"percentDone"`
		}
		if err := json.Unmarshal(row, &item); err != nil || item.InfoHash == "" {
			continue
		}
		task := Task{InfoHash: item.InfoHash, Status: "unknown"}
		_ = json.Unmarshal(item.Name, &task.Name)
		if strings.Contains(task.InfoHash, accessToken) || strings.Contains(task.Name, accessToken) {
			return TaskPage{}, errInvalidReply
		}
		if status, ok := taskNumber(item.Status); ok {
			switch status {
			case -1:
				task.Status = "failed"
			case 0:
				task.Status = "pending"
			case 1:
				task.Status = "downloading"
			case 2:
				task.Status = "completed"
			}
		}
		if progress, ok := taskNumber(item.Percent); ok {
			task.Progress = int(max(0, min(progress, 100)))
		}
		p.Tasks = append(p.Tasks, task)
	}
	if len(data.Tasks) > 0 && len(p.Tasks) == 0 {
		return TaskPage{}, errInvalidReply
	}
	return p, nil
}

func taskNumber(raw json.RawMessage) (float64, bool) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	value, err := n.Float64()
	return value, err == nil
}

type result struct {
	State json.RawMessage `json:"state"`
	Code  *int            `json:"code"`
	Errno int             `json:"errno"`
}

func (r result) check() error {
	if r.Errno != 0 {
		return businessError(r.Errno)
	}
	if r.Code == nil {
		return errInvalidReply
	}
	if *r.Code != 0 {
		return businessError(*r.Code)
	}
	if string(r.State) != "true" && string(r.State) != "1" {
		return errInvalidReply
	}
	return nil
}

func businessError(code int) error {
	switch code {
	case 401, 403, 40140116, 40140123, 40140124, 40140125, 40140126:
		return ErrUnauthorized
	case 429, 590075, 40140117:
		return ErrRateLimited
	default:
		return fmt.Errorf("115 拒绝了请求（错误码 %d）", code)
	}
}

func (c *Client) request(ctx context.Context, token, method, endpoint string, body io.Reader) (json.RawMessage, error) {
	token = strings.TrimSpace(token)
	if token == "" || strings.IndexFunc(token, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return nil, ErrUnauthorized
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+endpoint, body)
	if err != nil {
		return nil, errors.New("无法创建 115 请求")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("无法连接 115 开放平台")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("115 返回 HTTP %d", resp.StatusCode)
	}
	const maxResponseSize = 4 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil || len(raw) > maxResponseSize {
		return nil, errInvalidReply
	}
	var envelope struct {
		result
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, errInvalidReply
	}
	if err := envelope.check(); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}
