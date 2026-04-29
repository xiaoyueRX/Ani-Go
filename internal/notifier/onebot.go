package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// OneBotNotifier OneBot 协议通知器，支持 NapCat / go-cqhttp / Lagrange / LLOneBot
// 默认使用 HTTP API 模式（NapCat 默认端口 3000）

type OneBotNotifier struct {
	httpClient *http.Client
	host       string
	token      string
	userID     int64  // 私聊目标 QQ
	groupID    int64  // 群聊目标群号
}

func NewOneBotNotifier(host, token string, userID, groupID int64) *OneBotNotifier {
	if host == "" {
		host = "http://localhost:3000"
	}
	host = strings.TrimSuffix(host, "/")
	return &OneBotNotifier{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		host:       host,
		token:      token,
		userID:     userID,
		groupID:    groupID,
	}
}

func (ob *OneBotNotifier) Name() string { return "QQ(OneBot)" }

func (ob *OneBotNotifier) Send(ctx context.Context, title, message string) error {
	text := fmt.Sprintf("%s\n%s", title, message)
	var errs []string

	if ob.userID != 0 {
		if err := ob.sendPrivateMsg(ctx, ob.userID, text); err != nil {
			errs = append(errs, fmt.Sprintf("私聊失败: %v", err))
		} else {
			log.Printf("🔔 [QQ] 私聊通知已发送: %s", title)
		}
	}

	if ob.groupID != 0 {
		if err := ob.sendGroupMsg(ctx, ob.groupID, text); err != nil {
			errs = append(errs, fmt.Sprintf("群聊失败: %v", err))
		} else {
			log.Printf("🔔 [QQ] 群聊通知已发送: %s", title)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("[QQ] %s", strings.Join(errs, "; "))
	}
	if ob.userID == 0 && ob.groupID == 0 {
		return fmt.Errorf("[QQ] 未设置 user_id 或 group_id")
	}
	return nil
}

func (ob *OneBotNotifier) sendPrivateMsg(ctx context.Context, userID int64, message string) error {
	return ob.callAPI(ctx, "/send_private_msg", map[string]interface{}{
		"user_id": userID,
		"message": message,
	})
}

func (ob *OneBotNotifier) sendGroupMsg(ctx context.Context, groupID int64, message string) error {
	return ob.callAPI(ctx, "/send_group_msg", map[string]interface{}{
		"group_id": groupID,
		"message":  message,
	})
}

func (ob *OneBotNotifier) callAPI(ctx context.Context, endpoint string, params map[string]interface{}) error {
	payload, _ := json.Marshal(params)
	url := ob.host + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if ob.token != "" {
		req.Header.Set("Authorization", "Bearer "+ob.token)
	}

	resp, err := ob.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("返回状态码 %d", resp.StatusCode)
	}
	return nil
}
