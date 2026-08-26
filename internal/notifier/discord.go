package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

type DiscordNotifier struct {
	httpClient *http.Client
	webhookURL string
}

func NewDiscordNotifier(webhookURL string) *DiscordNotifier {
	return &DiscordNotifier{
		httpClient: httpx.New(10 * time.Second),
		webhookURL: webhookURL,
	}
}

func (d *DiscordNotifier) Name() string { return "Discord" }

func (d *DiscordNotifier) Send(ctx context.Context, title, message string) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       title,
				"description": message,
				"color":       3447003, // Blue
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[Discord] 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[Discord] 发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("[Discord] 返回状态码 %d", resp.StatusCode)
	}

	log.Printf("🔔 [Discord] 通知已发送: %s", title)
	return nil
}
