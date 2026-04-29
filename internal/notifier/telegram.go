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

type TelegramNotifier struct {
	httpClient *http.Client
	botToken   string
	chatID     string
}

func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	return &TelegramNotifier{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		botToken:   botToken,
		chatID:     chatID,
	}
}

func (t *TelegramNotifier) Name() string { return "Telegram" }

func (t *TelegramNotifier) Send(ctx context.Context, title, message string) error {
	text := fmt.Sprintf("*%s*\n%s", title, message)
	payload, _ := json.Marshal(map[string]string{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[Telegram] 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[Telegram] 发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("[Telegram] 返回状态码 %d", resp.StatusCode)
	}

	log.Printf("🔔 [Telegram] 通知已发送: %s", title)
	return nil
}
