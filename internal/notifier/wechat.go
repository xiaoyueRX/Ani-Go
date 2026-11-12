package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

const defaultWeChatBaseURL = "https://api.weixin.qq.com"

// WeChatNotifier 微信公众号消息通知器（支持客服消息与模板消息双模式）
type WeChatNotifier struct {
	AppID       string
	AppSecret   string
	UserID      string
	TemplateID  string // 可选：公众号模板消息 ID；为空时使用客服文本消息接口
	BaseURL     string // 可选：API 基础地址，默认 https://api.weixin.qq.com
	httpClient  *http.Client
	accessToken string
	tokenExpiry time.Time
	mu          sync.Mutex
}

// NewWeChatNotifier 从环境变量创建微信公众号通知器
func NewWeChatNotifier() *WeChatNotifier {
	return NewWeChatNotifierWithParams(
		os.Getenv("WECHAT_APP_ID"),
		os.Getenv("WECHAT_APP_SECRET"),
		os.Getenv("WECHAT_USER_ID"),
		os.Getenv("WECHAT_TEMPLATE_ID"),
	)
}

// NewWeChatNotifierWithParams 使用显式参数创建微信公众号通知器
func NewWeChatNotifierWithParams(appID, appSecret, userID, templateID string) *WeChatNotifier {
	return &WeChatNotifier{
		AppID:      strings.TrimSpace(appID),
		AppSecret:  strings.TrimSpace(appSecret),
		UserID:     strings.TrimSpace(userID),
		TemplateID: strings.TrimSpace(templateID),
		BaseURL:    defaultWeChatBaseURL,
		httpClient: httpx.Default,
	}
}

// Name 返回通知渠道名称
func (n *WeChatNotifier) Name() string {
	return "WeChat"
}

func (n *WeChatNotifier) apiBaseURL() string {
	base := strings.TrimRight(strings.TrimSpace(n.BaseURL), "/")
	if base == "" {
		return defaultWeChatBaseURL
	}
	return base
}

func (n *WeChatNotifier) client() *http.Client {
	if n.httpClient != nil {
		return n.httpClient
	}
	return httpx.Default
}

// getAccessToken 获取并缓存公众号 AccessToken，forceRefresh 为 true 时强制刷新
func (n *WeChatNotifier) getAccessToken(ctx context.Context, forceRefresh bool) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !forceRefresh && n.accessToken != "" && time.Now().Before(n.tokenExpiry) {
		return n.accessToken, nil
	}

	appID := strings.TrimSpace(n.AppID)
	appSecret := strings.TrimSpace(n.AppSecret)
	endpoint := fmt.Sprintf(
		"%s/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s",
		n.apiBaseURL(),
		url.QueryEscape(appID),
		url.QueryEscape(appSecret),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("wechat 创建获取 token 请求失败: %w", err)
	}

	resp, err := n.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("wechat 获取 token 请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("wechat 读取 token 响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("wechat 获取 token 返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("wechat 解析 token 响应失败: %w", err)
	}

	if result.ErrCode != 0 {
		return "", fmt.Errorf("wechat token 接口返回错误: %d - %s", result.ErrCode, result.ErrMsg)
	}
	if strings.TrimSpace(result.AccessToken) == "" {
		return "", fmt.Errorf("wechat token 接口返回空 access_token")
	}

	n.accessToken = result.AccessToken
	n.tokenExpiry = computeTokenExpiry(result.ExpiresIn)
	return n.accessToken, nil
}

// computeTokenExpiry 保护 ExpiresIn <= 60 的边界情况，避免算出负过期时间
func computeTokenExpiry(expiresIn int) time.Time {
	now := time.Now()
	if expiresIn > 60 {
		// 提前 60 秒刷新，避免临界过期
		return now.Add(time.Duration(expiresIn-60) * time.Second)
	}
	if expiresIn > 0 {
		// 短有效期时取一半作为安全缓存窗口，避免立即失效或负数
		return now.Add(time.Duration(expiresIn/2) * time.Second)
	}
	// 非正数不缓存，下次请求直接重新获取
	return now
}

func (n *WeChatNotifier) invalidateToken(failedToken string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.accessToken == failedToken {
		n.accessToken = ""
		n.tokenExpiry = time.Time{}
	}
}

func isWeChatTokenExpiredCode(errCode int) bool {
	return errCode == 40001 || errCode == 42001 || errCode == 40014
}

// Send 发送微信公众号消息（支持 token 失效自动刷新并重试一次）
func (n *WeChatNotifier) Send(ctx context.Context, title, message string) error {
	appID := strings.TrimSpace(n.AppID)
	appSecret := strings.TrimSpace(n.AppSecret)
	userID := strings.TrimSpace(n.UserID)
	if appID == "" || appSecret == "" || userID == "" {
		return fmt.Errorf("wechat 凭证未配置完整（需要 AppID、AppSecret 与 UserID）")
	}

	token, err := n.getAccessToken(ctx, false)
	if err != nil {
		return err
	}

	errCode, err := n.sendWithToken(ctx, token, userID, title, message)
	if err == nil {
		return nil
	}

	// 遇到微信 access_token 失效/过期错误码时自动刷新并重试一次
	if isWeChatTokenExpiredCode(errCode) {
		n.invalidateToken(token)
		newToken, refreshErr := n.getAccessToken(ctx, true)
		if refreshErr != nil {
			return fmt.Errorf("wechat token 刷新失败: %w (原错误: %v)", refreshErr, err)
		}
		_, retryErr := n.sendWithToken(ctx, newToken, userID, title, message)
		return retryErr
	}

	return err
}

func (n *WeChatNotifier) sendWithToken(ctx context.Context, token, userID, title, message string) (int, error) {
	templateID := strings.TrimSpace(n.TemplateID)
	var endpoint string
	var payload map[string]interface{}

	if templateID != "" {
		// 微信公众号模板消息接口
		endpoint = fmt.Sprintf("%s/cgi-bin/message/template/send?access_token=%s", n.apiBaseURL(), url.QueryEscape(token))
		payload = map[string]interface{}{
			"touser":      userID,
			"template_id": templateID,
			"data": map[string]interface{}{
				"first": map[string]string{
					"value": title,
				},
				"keyword1": map[string]string{
					"value": title,
				},
				"keyword2": map[string]string{
					"value": message,
				},
				"remark": map[string]string{
					"value": message,
				},
			},
		}
	} else {
		// 微信公众号客服文本消息接口
		endpoint = fmt.Sprintf("%s/cgi-bin/message/custom/send?access_token=%s", n.apiBaseURL(), url.QueryEscape(token))
		content := message
		if strings.TrimSpace(title) != "" {
			content = fmt.Sprintf("【%s】\n%s", title, message)
		}
		payload = map[string]interface{}{
			"touser":  userID,
			"msgtype": "text",
			"text": map[string]string{
				"content": content,
			},
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("wechat 序列化消息失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("wechat 创建发送请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client().Do(req)
	if err != nil {
		return 0, fmt.Errorf("wechat 发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, fmt.Errorf("wechat 读取发送响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("wechat 发送返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return 0, fmt.Errorf("wechat 解析发送响应失败: %w", err)
	}

	if result.ErrCode != 0 {
		return result.ErrCode, fmt.Errorf("wechat 发送接口返回错误: %d - %s", result.ErrCode, result.ErrMsg)
	}

	return 0, nil
}
