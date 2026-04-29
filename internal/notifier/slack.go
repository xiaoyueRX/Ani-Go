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

type SlackNotifier struct {
	httpClient *http.Client
	webhookURL string
}

func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		webhookURL: webhookURL,
	}
}

func (s *SlackNotifier) Name() string { return "Slack" }

func (s *SlackNotifier) Send(ctx context.Context, title, message string) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"blocks": []map[string]interface{}{
			{
				"type": "header",
				"text": map[string]string{"type": "plain_text", "text": title},
			},
			{
				"type": "section",
				"text": map[string]string{"type": "mrkdwn", "text": message},
			},
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[Slack] 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[Slack] 发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("[Slack] 返回状态码 %d", resp.StatusCode)
	}

	log.Printf("🔔 [Slack] 通知已发送: %s", title)
	return nil
}
