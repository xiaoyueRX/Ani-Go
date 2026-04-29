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

type WhatsAppNotifier struct {
	httpClient    *http.Client
	phoneNumberID string
	accessToken   string
	to            string
}

func NewWhatsAppNotifier(phoneNumberID, accessToken, to string) *WhatsAppNotifier {
	return &WhatsAppNotifier{
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		phoneNumberID: phoneNumberID,
		accessToken:   accessToken,
		to:            to,
	}
}

func (w *WhatsAppNotifier) Name() string { return "WhatsApp" }

func (w *WhatsAppNotifier) Send(ctx context.Context, title, message string) error {
	text := fmt.Sprintf("*[Ani-Go] %s*\n%s", title, message)
	payload, _ := json.Marshal(map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                w.to,
		"type":              "text",
		"text": map[string]string{
			"body": text,
		},
	})

	url := fmt.Sprintf("https://graph.facebook.com/v22.0/%s/messages", w.phoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[WhatsApp] 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+w.accessToken)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[WhatsApp] 发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("[WhatsApp] 返回状态码 %d", resp.StatusCode)
	}

	log.Printf("🔔 [WhatsApp] 通知已发送: %s", title)
	return nil
}
