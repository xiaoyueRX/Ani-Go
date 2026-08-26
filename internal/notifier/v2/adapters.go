package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

// ============================================================
// 统一 Notifier 接口
// ============================================================
type Notifier interface {
	Name() string
	Send(ctx context.Context, title, message string) error
}

// ============================================================
// Telegram 适配器
// ============================================================
type TelegramNotifier struct {
	httpClient *http.Client
	botToken   string
	chatID     string
}

func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	return &TelegramNotifier{
		httpClient: httpx.New(10 * time.Second),
		botToken:   botToken,
		chatID:     chatID,
	}
}

func (t *TelegramNotifier) Name() string { return "Telegram" }

func (t *TelegramNotifier) Send(ctx context.Context, title, message string) error {
	if t.botToken == "" || t.chatID == "" {
		return fmt.Errorf("telegram token or chat_id not configured")
	}

	// 支持 MarkdownV2，特殊字符需转义
	text := fmt.Sprintf("*%s*\n%s", t.escapeMarkdownV2(title), t.escapeMarkdownV2(message))
	payload, _ := json.Marshal(map[string]string{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "MarkdownV2",
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

func (t *TelegramNotifier) escapeMarkdownV2(s string) string {
	// MarkdownV2 特殊字符: _ * [ ] ( ) ~ ` > # + - = | { } . !
	replacer := strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]",
		"(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`",
		">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
		"=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}",
		".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}

// ============================================================
// 钉钉 适配器 (Webhook)
// ============================================================
type DingTalkNotifier struct {
	httpClient *http.Client
	webhookURL string
	secret     string // 可选：加签密钥
}

func NewDingTalkNotifier(webhookURL, secret string) *DingTalkNotifier {
	return &DingTalkNotifier{
		httpClient: httpx.New(10 * time.Second),
		webhookURL: webhookURL,
		secret:     secret,
	}
}

func (d *DingTalkNotifier) Name() string { return "钉钉" }

func (d *DingTalkNotifier) Send(ctx context.Context, title, message string) error {
	if d.webhookURL == "" {
		return fmt.Errorf("dingtalk webhook URL not configured")
	}

	url := d.webhookURL
	if d.secret != "" {
		timestamp := time.Now().UnixMilli()
		// 实际应用中需用 HMAC-SHA256 计算签名，这里简化
		url = fmt.Sprintf("%s&timestamp=%d&sign=TODO", url, timestamp)
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  fmt.Sprintf("### %s\n%s", title, message),
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[钉钉] 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[钉钉] 发送失败: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Errcode != 0 {
		return fmt.Errorf("[钉钉] 返回错误: %d - %s", result.Errcode, result.Errmsg)
	}

	log.Printf("🔔 [钉钉] 通知已发送: %s", title)
	return nil
}

// ============================================================
// 企业微信 适配器 (Webhook)
// ============================================================
type WeComNotifier struct {
	httpClient *http.Client
	webhookURL string
}

func NewWeComNotifier(webhookURL string) *WeComNotifier {
	return &WeComNotifier{
		httpClient: httpx.New(10 * time.Second),
		webhookURL: webhookURL,
	}
}

func (w *WeComNotifier) Name() string { return "企业微信" }

func (w *WeComNotifier) Send(ctx context.Context, title, message string) error {
	if w.webhookURL == "" {
		return fmt.Errorf("wecom webhook URL not configured")
	}

	// 企业微信 Markdown 支持有限，用 text 类型兼容性最好
	payload, _ := json.Marshal(map[string]interface{}{
		"msgtype": "text",
		"text": map[string]interface{}{
			"content": fmt.Sprintf("%s\n%s", title, message),
			"mentioned_list": []string{"@all"},
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[企业微信] 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[企业微信] 发送失败: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Errcode != 0 {
		return fmt.Errorf("[企业微信] 返回错误: %d - %s", result.Errcode, result.Errmsg)
	}

	log.Printf("🔔 [企业微信] 通知已发送: %s", title)
	return nil
}

// ============================================================
// 飞书 适配器 (Webhook)
// ============================================================
type FeishuNotifier struct {
	httpClient *http.Client
	webhookURL string
}

func NewFeishuNotifier(webhookURL string) *FeishuNotifier {
	return &FeishuNotifier{
		httpClient: httpx.New(10 * time.Second),
		webhookURL: webhookURL,
	}
}

func (f *FeishuNotifier) Name() string { return "飞书" }

func (f *FeishuNotifier) Send(ctx context.Context, title, message string) error {
	if f.webhookURL == "" {
		return fmt.Errorf("feishu webhook URL not configured")
	}

	// 飞书支持 Post 富文本 / Interactive Card
	payload, _ := json.Marshal(map[string]interface{}{
		"msg_type": "post",
		"content": map[string]interface{}{
			"post": map[string]interface{}{
				"zh_cn": map[string]interface{}{
					"title": title,
					"content": [][]map[string]string{
						{{"tag": "text", "text": message}},
					},
				},
			},
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("[飞书] 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("[飞书] 发送失败: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Code != 0 {
		return fmt.Errorf("[飞书] 返回错误: %d - %s", result.Code, result.Msg)
	}

	log.Printf("🔔 [飞书] 通知已发送: %s", title)
	return nil
}

// ============================================================
// OneBot (QQ) 适配器
// ============================================================
type OneBotNotifier struct {
	httpClient *http.Client
	host       string
	token      string
	userID     int64
	groupID    int64
	mu         sync.Mutex
}

func NewOneBotNotifier(host, token string, userID, groupID int64) *OneBotNotifier {
	if host == "" {
		host = "http://localhost:3000"
	}
	host = strings.TrimSuffix(host, "/")
	return &OneBotNotifier{
		httpClient: httpx.New(10 * time.Second),
		host:       host,
		token:      token,
		userID:     userID,
		groupID:    groupID,
	}
}

func (o *OneBotNotifier) Name() string { return "QQ(OneBot)" }

func (o *OneBotNotifier) Send(ctx context.Context, title, message string) error {
	text := fmt.Sprintf("%s\n%s", title, message)
	var errs []string

	if o.userID != 0 {
		if err := o.sendPrivateMsg(ctx, o.userID, text); err != nil {
			errs = append(errs, fmt.Sprintf("私聊失败: %v", err))
		} else {
			log.Printf("🔔 [QQ] 私聊通知已发送: %s", title)
		}
	}

	if o.groupID != 0 {
		if err := o.sendGroupMsg(ctx, o.groupID, text); err != nil {
			errs = append(errs, fmt.Sprintf("群聊失败: %v", err))
		} else {
			log.Printf("🔔 [QQ] 群聊通知已发送: %s", title)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("[QQ] %s", strings.Join(errs, "; "))
	}
	if o.userID == 0 && o.groupID == 0 {
		return fmt.Errorf("[QQ] 未设置 user_id 或 group_id")
	}
	return nil
}

func (o *OneBotNotifier) sendPrivateMsg(ctx context.Context, userID int64, message string) error {
	return o.callAPI(ctx, "/send_private_msg", map[string]interface{}{
		"user_id": userID,
		"message": message,
	})
}

func (o *OneBotNotifier) sendGroupMsg(ctx context.Context, groupID int64, message string) error {
	return o.callAPI(ctx, "/send_group_msg", map[string]interface{}{
		"group_id": groupID,
		"message":  message,
	})
}

func (o *OneBotNotifier) callAPI(ctx context.Context, endpoint string, params map[string]interface{}) error {
	payload, _ := json.Marshal(params)
	url := o.host + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if o.token != "" {
		req.Header.Set("Authorization", "Bearer "+o.token)
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("返回状态码 %d", resp.StatusCode)
	}
	return nil
}

// ============================================================
// 通知器工厂：从配置创建所有已配置的 Notifier
// ============================================================
type NotifierConfig struct {
	// Telegram
	TelegramBotToken string
	TelegramChatID   string
	// 钉钉
	DingTalkWebhook string
	DingTalkSecret  string
	// 企业微信
	WeComWebhook string
	// 飞书
	FeishuWebhook string
	// OneBot (QQ)
	OneBotHost   string
	OneBotToken  string
	OneBotUserID int64
	OneBotGroupID int64
}

func CreateNotifiersFromConfig(cfg NotifierConfig) []Notifier {
	var notifiers []Notifier

	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		notifiers = append(notifiers, NewTelegramNotifier(cfg.TelegramBotToken, cfg.TelegramChatID))
		log.Printf("✅ Telegram 通知器已就绪")
	}

	if cfg.DingTalkWebhook != "" {
		notifiers = append(notifiers, NewDingTalkNotifier(cfg.DingTalkWebhook, cfg.DingTalkSecret))
		log.Printf("✅ 钉钉 通知器已就绪")
	}

	if cfg.WeComWebhook != "" {
		notifiers = append(notifiers, NewWeComNotifier(cfg.WeComWebhook))
		log.Printf("✅ 企业微信 通知器已就绪")
	}

	if cfg.FeishuWebhook != "" {
		notifiers = append(notifiers, NewFeishuNotifier(cfg.FeishuWebhook))
		log.Printf("✅ 飞书 通知器已就绪")
	}

	if cfg.OneBotHost != "" && (cfg.OneBotUserID != 0 || cfg.OneBotGroupID != 0) {
		notifiers = append(notifiers, NewOneBotNotifier(cfg.OneBotHost, cfg.OneBotToken, cfg.OneBotUserID, cfg.OneBotGroupID))
		log.Printf("✅ QQ(OneBot) 通知器已就绪")
	}

	return notifiers
}