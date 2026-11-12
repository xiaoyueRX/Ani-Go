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

	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

// SignalNotifier 基于 signal-cli-rest-api 的 Signal 消息通知器
type SignalNotifier struct {
	APIURL     string
	Sender     string
	Recipients []string
	httpClient *http.Client
}

// NewSignalNotifier 从环境变量创建 Signal 通知器
func NewSignalNotifier() *SignalNotifier {
	return NewSignalNotifierWithParams(
		os.Getenv("SIGNAL_API_URL"),
		os.Getenv("SIGNAL_SENDER"),
		splitAndTrimRecipients(os.Getenv("SIGNAL_RECIPIENTS")),
	)
}

// NewSignalNotifierWithParams 使用显式参数创建 Signal 通知器
func NewSignalNotifierWithParams(apiURL, sender string, recipients []string) *SignalNotifier {
	return &SignalNotifier{
		APIURL:     strings.TrimRight(strings.TrimSpace(apiURL), "/"),
		Sender:     strings.TrimSpace(sender),
		Recipients: normalizeSignalRecipients(recipients),
		httpClient: httpx.Default,
	}
}

// Name 返回通知渠道名称
func (n *SignalNotifier) Name() string {
	return "Signal"
}

// Send 发送 Signal 通知消息
func (n *SignalNotifier) Send(ctx context.Context, title, message string) error {
	apiURL := strings.TrimRight(strings.TrimSpace(n.APIURL), "/")
	sender := strings.TrimSpace(n.Sender)
	recipients := normalizeSignalRecipients(n.Recipients)

	if apiURL == "" || sender == "" || len(recipients) == 0 {
		return fmt.Errorf("signal 配置不完整（需要 API URL、发送者号码与至少一个接收者）")
	}

	parsedURL, err := url.Parse(apiURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return fmt.Errorf("signal API URL 无效: %q", n.APIURL)
	}

	endpoint := fmt.Sprintf("%s/v2/send", apiURL)
	content := message
	if strings.TrimSpace(title) != "" {
		content = fmt.Sprintf("【%s】\n%s", title, message)
	}

	payload := map[string]interface{}{
		"message":    content,
		"number":     sender,
		"recipients": recipients,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("signal 序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("signal 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := n.httpClient
	if client == nil {
		client = httpx.Default
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("signal 发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		snippetBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		snippet := strings.TrimSpace(string(snippetBytes))
		if snippet != "" {
			return fmt.Errorf("signal API 返回错误状态码 %d: %s", resp.StatusCode, snippet)
		}
		return fmt.Errorf("signal API 返回错误状态码: %d", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	return nil
}

func splitAndTrimRecipients(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return normalizeSignalRecipients(strings.Split(raw, ","))
}

func normalizeSignalRecipients(recipients []string) []string {
	if len(recipients) == 0 {
		return nil
	}
	cleaned := make([]string, 0, len(recipients))
	seen := make(map[string]struct{}, len(recipients))
	for _, r := range recipients {
		trimmed := strings.TrimSpace(r)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		cleaned = append(cleaned, trimmed)
	}
	return cleaned
}
