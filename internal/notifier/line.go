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

type LINENotifier struct {
	httpClient *http.Client
	channelToken string
	userID       string
}

func NewLINENotifier(channelToken, userID string) *LINENotifier {
	return &LINENotifier{
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		channelToken: channelToken,
		userID:       userID,
	}
}

func (l *LINENotifier) Name() string { return "LINE" }

func (l *LINENotifier) Send(ctx context.Context, title, message string) error {
	text := fmt.Sprintf("[Ani-Go] %s\n%s", title, message)
	payload, _ := json.Marshal(map[string]interface{}{
		"to": l.userID,
		"messages": []map[string]string{
			{"type": "text", "text": text},
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.line.me/v2/bot/message/push", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[LINE] 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+l.channelToken)

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[LINE] 发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("[LINE] 返回状态码 %d", resp.StatusCode)
	}

	log.Printf("🔔 [LINE] 通知已发送: %s", title)
	return nil
}
