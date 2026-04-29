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

type MatrixNotifier struct {
	httpClient  *http.Client
	homeserver  string
	accessToken string
	roomID      string
}

func NewMatrixNotifier(homeserver, accessToken, roomID string) *MatrixNotifier {
	return &MatrixNotifier{
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		homeserver:  homeserver,
		accessToken: accessToken,
		roomID:      roomID,
	}
}

func (m *MatrixNotifier) Name() string { return "Matrix" }

func (m *MatrixNotifier) Send(ctx context.Context, title, message string) error {
	text := fmt.Sprintf("%s\n%s", title, message)
	payload, _ := json.Marshal(map[string]string{
		"msgtype": "m.text",
		"body":    text,
	})

	txnID := fmt.Sprintf("anigo-%d", time.Now().UnixNano())
	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		m.homeserver, m.roomID, txnID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[Matrix] 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+m.accessToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[Matrix] 发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("[Matrix] 返回状态码 %d", resp.StatusCode)
	}

	log.Printf("🔔 [Matrix] 通知已发送: %s", title)
	return nil
}
