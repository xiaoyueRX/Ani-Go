package notifier

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	v2 "github.com/xiaoyueRX/Ani-Go/internal/notifier/v2"
)

// BuildAllNotifiers 根据提供的 getter 函数读取设置，创建所有已配置的通知器列表
func BuildAllNotifiers(get func(key string) string) []v2.Notifier {
	var list []v2.Notifier

	// 1. Telegram
	tgToken := get("TELEGRAM_BOT_TOKEN")
	tgChatID := get("TELEGRAM_CHAT_ID")
	if tgToken != "" && tgChatID != "" {
		list = append(list, v2.NewTelegramNotifier(tgToken, tgChatID))
	}

	// 2. 钉钉
	dingWebhook := get("DINGTALK_WEBHOOK")
	if dingWebhook != "" {
		list = append(list, v2.NewDingTalkNotifier(dingWebhook, get("DINGTALK_SECRET")))
	}

	// 3. 企业微信
	wecomWebhook := get("WECOM_WEBHOOK")
	if wecomWebhook != "" {
		list = append(list, v2.NewWeComNotifier(wecomWebhook))
	}

	// 4. 飞书
	feishuWebhook := get("FEISHU_WEBHOOK")
	if feishuWebhook != "" {
		list = append(list, v2.NewFeishuNotifier(feishuWebhook))
	}

	// 5. OneBot (QQ)
	obHost := get("ONEBOT_HOST")
	obUID, _ := strconv.ParseInt(get("ONEBOT_USER_ID"), 10, 64)
	obGID, _ := strconv.ParseInt(get("ONEBOT_GROUP_ID"), 10, 64)
	if obHost != "" && (obUID != 0 || obGID != 0) {
		list = append(list, v2.NewOneBotNotifier(obHost, get("ONEBOT_TOKEN"), obUID, obGID))
	}

	// 6. Bark
	barkKey := get("BARK_DEVICE_KEY")
	if barkKey != "" {
		barkServer := get("BARK_SERVER_URL")
		if barkServer == "" {
			barkServer = "https://api.day.app"
		}
		endpoint := strings.TrimRight(barkServer, "/") + "/push"
		list = append(list, NewPushNotifier(PushBark, endpoint, barkKey, ""))
	}

	// 7. Server酱
	sckey := get("SERVERCHAN_KEY")
	if sckey != "" {
		list = append(list, NewPushNotifier(PushServerChan, "https://sctapi.ftqq.com/"+sckey+".send", "", ""))
	}

	// 8. Discord
	discordWebhook := get("DISCORD_WEBHOOK")
	if discordWebhook != "" {
		list = append(list, NewDiscordNotifier(discordWebhook))
	}

	// 9. Slack
	slackWebhook := get("SLACK_WEBHOOK")
	if slackWebhook != "" {
		list = append(list, NewSlackNotifier(slackWebhook))
	}

	// 10. Gotify
	gotifyURL := get("GOTIFY_URL")
	gotifyToken := get("GOTIFY_TOKEN")
	if gotifyURL != "" && gotifyToken != "" {
		list = append(list, NewPushNotifier(PushGotify, strings.TrimRight(gotifyURL, "/")+"/message?token="+gotifyToken, "", ""))
	}

	// 11. ntfy (必须配置 topic)
	ntfyTopic := get("NTFY_TOPIC")
	if ntfyTopic != "" {
		ntfyURL := get("NTFY_URL")
		if ntfyURL == "" {
			ntfyURL = "https://ntfy.sh"
		}
		endpoint := strings.TrimRight(ntfyURL, "/") + "/" + ntfyTopic
		list = append(list, NewPushNotifier(PushNtfy, endpoint, "", ""))
	}

	// 12. Pushover
	pushoverToken := get("PUSHOVER_TOKEN")
	pushoverUser := get("PUSHOVER_USER")
	if pushoverToken != "" && pushoverUser != "" {
		list = append(list, NewPushNotifier(PushPushover, "https://api.pushover.net/1/messages.json", pushoverToken, pushoverUser))
	}

	// 13. Email
	smtpHost := get("EMAIL_SMTP_HOST")
	smtpUser := get("EMAIL_SMTP_USER")
	smtpTo := get("EMAIL_SMTP_TO")
	if smtpHost != "" && smtpUser != "" && smtpTo != "" {
		smtpPort := get("EMAIL_SMTP_PORT")
		smtpPass := get("EMAIL_SMTP_PASS")
		smtpFrom := get("EMAIL_SMTP_FROM")
		if smtpFrom == "" {
			smtpFrom = smtpUser
		}
		var toList []string
		for _, item := range strings.Split(smtpTo, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				toList = append(toList, trimmed)
			}
		}
		list = append(list, NewEmailNotifier(smtpHost, smtpPort, smtpUser, smtpPass, smtpFrom, toList))
	}

	// 14. Matrix
	matrixServer := get("MATRIX_HOMESERVER")
	matrixToken := get("MATRIX_TOKEN")
	matrixRoom := get("MATRIX_ROOM_ID")
	if matrixServer != "" && matrixToken != "" && matrixRoom != "" {
		list = append(list, NewMatrixNotifier(matrixServer, matrixToken, matrixRoom))
	}

	// 15. LINE
	lineToken := get("LINE_CHANNEL_TOKEN")
	lineUser := get("LINE_USER_ID")
	if lineToken != "" && lineUser != "" {
		list = append(list, NewLINENotifier(lineToken, lineUser))
	}

	// 16. WhatsApp
	waPhoneID := get("WHATSAPP_PHONE_ID")
	waToken := get("WHATSAPP_TOKEN")
	waTo := get("WHATSAPP_TO")
	if waPhoneID != "" && waToken != "" && waTo != "" {
		list = append(list, NewWhatsAppNotifier(waPhoneID, waToken, waTo))
	}

	// 17. Signal
	signalURL := get("SIGNAL_API_URL")
	signalSender := get("SIGNAL_SENDER")
	signalRecipients := get("SIGNAL_RECIPIENTS")
	if signalURL != "" && signalSender != "" && signalRecipients != "" {
		var rList []string
		for _, r := range strings.Split(signalRecipients, ",") {
			if trimmed := strings.TrimSpace(r); trimmed != "" {
				rList = append(rList, trimmed)
			}
		}
		list = append(list, NewSignalNotifierWithParams(signalURL, signalSender, rList))
	}

	return list
}

// BuildAllNotifiersFromSettingsAndEnv 从数据库设置与环境变量构建全部已配置的通知器
func BuildAllNotifiersFromSettingsAndEnv() []v2.Notifier {
	notifiers := BuildAllNotifiers(func(key string) string {
		if database.DB != nil {
			var s database.Setting
			if err := database.DB.Where("key = ?", key).First(&s).Error; err == nil && s.Value != "" {
				return strings.TrimSpace(s.Value)
			}
		}
		return strings.TrimSpace(os.Getenv(key))
	})
	log.Printf("🔔 [通知中心] 已根据数据库与环境变量加载 %d 个通知渠道", len(notifiers))
	return notifiers
}
