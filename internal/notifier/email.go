package notifier

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"strings"
)

type EmailNotifier struct {
	smtpHost string
	smtpPort string
	username string
	password string
	from     string
	to       []string
}

func NewEmailNotifier(smtpHost, smtpPort, username, password, from string, to []string) *EmailNotifier {
	if smtpPort == "" {
		smtpPort = "587"
	}
	return &EmailNotifier{
		smtpHost: smtpHost,
		smtpPort: smtpPort,
		username: username,
		password: password,
		from:     from,
		to:       to,
	}
}

func (e *EmailNotifier) Name() string { return "Email" }

func (e *EmailNotifier) Send(ctx context.Context, title, message string) error {
	subject := fmt.Sprintf("[Ani-Go] %s", title)
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		e.from, strings.Join(e.to, ","), subject, message)

	addr := fmt.Sprintf("%s:%s", e.smtpHost, e.smtpPort)
	auth := smtp.PlainAuth("", e.username, e.password, e.smtpHost)

	done := make(chan error, 1)
	go func() {
		done <- smtp.SendMail(addr, auth, e.from, e.to, []byte(body))
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("[Email] 发送超时: %w", ctx.Err())
	case err := <-done:
		if err != nil {
			return fmt.Errorf("[Email] 发送失败: %w", err)
		}
		log.Printf("🔔 [Email] 通知已发送 → %s", strings.Join(e.to, ","))
		return nil
	}
}
