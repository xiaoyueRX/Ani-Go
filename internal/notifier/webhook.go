package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// WebhookType 定义不同平台的 Webhook 消息格式
type WebhookType string

const (
	WebhookDiscord WebhookType = "discord"
	WebhookWecom   WebhookType = "wecom"   // 企业微信
	WebhookFeishu  WebhookType = "feishu"  // 飞书
	WebhookDingTalk WebhookType = "dingtalk" // 钉钉
	WebhookGeneric WebhookType = "generic" // 通用 JSON
)

type WebhookNotifier struct {
	httpClient *http.Client
	url        string
	wtype      WebhookType
	name       string
}

func NewWebhookNotifier(url string, wtype WebhookType) *WebhookNotifier {
	return &WebhookNotifier{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		url:        url,
		wtype:      wtype,
		name:       string(wtype),
	}
}

func (w *WebhookNotifier) Name() string { return w.name }

func (w *WebhookNotifier) Send(ctx context.Context, title, message string) error {
	payload, err := w.buildPayload(title, message)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[%s] 创建请求失败: %w", w.name, err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[%s] 发送失败: %w", w.name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("[%s] 返回状态码 %d", w.name, resp.StatusCode)
	}

	log.Printf("🔔 [%s] 通知已发送: %s", w.name, title)
	return nil
}

func (w *WebhookNotifier) buildPayload(title, message string) ([]byte, error) {
	switch w.wtype {
	case WebhookDiscord:
		return json.Marshal(map[string]string{
			"content":  fmt.Sprintf("**%s**\n%s", title, message),
			"username": "Ani-Go",
		})
	case WebhookWecom:
		return json.Marshal(map[string]interface{}{
			"msgtype": "text",
			"text":    map[string]string{"content": fmt.Sprintf("%s\n%s", title, message)},
		})
	case WebhookFeishu:
		return json.Marshal(map[string]interface{}{
			"msg_type": "text",
			"content":  map[string]string{"text": fmt.Sprintf("%s\n%s", title, message)},
		})
	case WebhookDingTalk:
		return json.Marshal(map[string]interface{}{
			"msgtype": "text",
			"text":    map[string]string{"content": fmt.Sprintf("%s\n%s", title, message)},
		})
	default:
		return json.Marshal(map[string]string{
			"title":   title,
			"message": message,
		})
	}
}
