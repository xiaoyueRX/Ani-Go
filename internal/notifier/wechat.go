package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

type WeChatNotifier struct {
	AppID       string
	AppSecret   string
	UserID      string
	accessToken string
	tokenExpiry time.Time
	mu          sync.Mutex
}

func NewWeChatNotifier() *WeChatNotifier {
	return &WeChatNotifier{
		AppID:     os.Getenv("WECHAT_APP_ID"),
		AppSecret: os.Getenv("WECHAT_APP_SECRET"),
		UserID:    os.Getenv("WECHAT_USER_ID"),
	}
}

func (n *WeChatNotifier) Name() string {
	return "WeChat"
}

func (n *WeChatNotifier) getAccessToken(ctx context.Context) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.accessToken != "" && time.Now().Before(n.tokenExpiry) {
		return n.accessToken, nil
	}

	url := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s", n.AppID, n.AppSecret)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.ErrCode != 0 {
		return "", fmt.Errorf("wechat api error: %d - %s", result.ErrCode, result.ErrMsg)
	}

	n.accessToken = result.AccessToken
	// Reserve 60 seconds buffer before expiry
	n.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)
	return n.accessToken, nil
}

func (n *WeChatNotifier) Send(ctx context.Context, title, message string) error {
	if n.AppID == "" || n.AppSecret == "" || n.UserID == "" {
		return fmt.Errorf("wechat credentials not configured")
	}

	token, err := n.getAccessToken(ctx)
	if err != nil {
		return err
	}

	// Using Customer Service Message API
	url := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/message/custom/send?access_token=%s", token)

	payload := map[string]interface{}{
		"touser":  n.UserID,
		"msgtype": "text",
		"text": map[string]string{
			"content": fmt.Sprintf("【%s】\n%s", title, message),
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.ErrCode != 0 {
		return fmt.Errorf("wechat send error: %d - %s", result.ErrCode, result.ErrMsg)
	}

	return nil
}
