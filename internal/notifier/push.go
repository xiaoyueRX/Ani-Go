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

// PushType 推送服务类型
type PushType string

const (
	PushServerChan PushType = "serverchan" // Server酱（微信推送）
	PushBark       PushType = "bark"       // Bark（iOS 推送）
	PushPushover   PushType = "pushover"   // Pushover
	PushGotify     PushType = "gotify"     // Gotify（自托管）
	PushNtfy       PushType = "ntfy"       // ntfy（自托管开源）
)

// PushNotifier 通用推送通知器，支持多种 Push 服务

type PushNotifier struct {
	httpClient *http.Client
	url        string
	titleParam string // 标题字段名（各服务不同）
	bodyParam  string // 正文字段名
	token      string // 额外认证 token
	userKey    string // Pushover user key
	pushtype   PushType
}

func NewPushNotifier(pushType PushType, url, token, userKey string) *PushNotifier {
	pn := &PushNotifier{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		url:        url,
		token:      token,
		userKey:    userKey,
		pushtype:   pushType,
	}

	switch pushType {
	case PushServerChan:
		// Server酱: POST https://sctapi.ftqq.com/{sendkey}.send
		// Body: {"title": "...", "desp": "..."}
		pn.titleParam = "title"
		pn.bodyParam = "desp"
	case PushBark:
		// Bark: POST https://api.day.app/{device_key}
		// Body: {"title": "...", "body": "...", "device_key": "..."}
		pn.titleParam = "title"
		pn.bodyParam = "body"
	case PushPushover:
		// Pushover: POST https://api.pushover.net/1/messages.json
		// Body: {"token": "...", "user": "...", "title": "...", "message": "..."}
		pn.titleParam = "title"
		pn.bodyParam = "message"
	case PushGotify:
		// Gotify: POST https://gotify.example.com/message?token=xxx
		// Body: {"title": "...", "message": "...", "priority": 5}
		pn.titleParam = "title"
		pn.bodyParam = "message"
	case PushNtfy:
		// ntfy: POST https://ntfy.sh/{topic}
		// Body: "message text" (raw), headers: Title, Priority
		pn.titleParam = "title"
		pn.bodyParam = "message"
	}

	return pn
}

func (pn *PushNotifier) Name() string { return string(pn.pushtype) }

func (pn *PushNotifier) Send(ctx context.Context, title, message string) error {
	switch pn.pushtype {
	case PushNtfy:
		return pn.sendNtfy(ctx, title, message)
	case PushPushover:
		return pn.sendPushover(ctx, title, message)
	default:
		return pn.sendJSON(ctx, title, message)
	}
}

func (pn *PushNotifier) sendJSON(ctx context.Context, title, message string) error {
	body := map[string]interface{}{
		pn.titleParam: title,
		pn.bodyParam:  message,
	}
	if pn.pushtype == PushGotify {
		body["priority"] = 5
	}
	if pn.pushtype == PushBark && pn.token != "" {
		body["device_key"] = pn.token
	}

	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pn.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[%s] 创建请求失败: %w", pn.pushtype, err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := pn.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[%s] 发送失败: %w", pn.pushtype, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("[%s] 返回状态码 %d", pn.pushtype, resp.StatusCode)
	}

	log.Printf("🔔 [%s] 通知已发送: %s", pn.pushtype, title)
	return nil
}

func (pn *PushNotifier) sendPushover(ctx context.Context, title, message string) error {
	payload, _ := json.Marshal(map[string]string{
		"token":   pn.token,
		"user":    pn.userKey,
		"title":   title,
		"message": message,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pn.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[Pushover] 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := pn.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[Pushover] 发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("[Pushover] 返回状态码 %d", resp.StatusCode)
	}

	log.Printf("🔔 [Pushover] 通知已发送: %s", title)
	return nil
}

func (pn *PushNotifier) sendNtfy(ctx context.Context, title, message string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pn.url, bytes.NewReader([]byte(message)))
	if err != nil {
		return fmt.Errorf("[ntfy] 创建请求失败: %w", err)
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", "default")

	resp, err := pn.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[ntfy] 发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("[ntfy] 返回状态码 %d", resp.StatusCode)
	}

	log.Printf("🔔 [ntfy] 通知已发送: %s", title)
	return nil
}
