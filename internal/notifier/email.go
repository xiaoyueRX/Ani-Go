package notifier

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"
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

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	addr := net.JoinHostPort(e.smtpHost, e.smtpPort)
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("[Email] 连接失败: %w", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	return e.sendOnConnection(conn, body, title)
}

// sendOnConnection 根据配置选择隐式 TLS 或 STARTTLS 并完成一次 SMTP 事务。
func (e *EmailNotifier) sendOnConnection(conn net.Conn, body, title string) error {
	tlsConfig := &tls.Config{ServerName: e.smtpHost, MinVersion: tls.VersionTLS12}
	var smtpConn net.Conn = conn
	if e.smtpPort == "465" {
		smtpConn = tls.Client(conn, tlsConfig)
	}
	client, err := smtp.NewClient(smtpConn, e.smtpHost)
	if err != nil {
		return fmt.Errorf("[Email] 建立 SMTP 会话失败: %w", err)
	}
	defer client.Close()
	if e.smtpPort != "465" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("[Email] STARTTLS 握手失败: %w", err)
			}
		} else if e.smtpPort == "587" {
			return fmt.Errorf("[Email] SMTP 587 服务未提供 STARTTLS")
		}
	}
	if e.username != "" {
		if err := client.Auth(smtp.PlainAuth("", e.username, e.password, e.smtpHost)); err != nil {
			return fmt.Errorf("[Email] 认证失败: %w", err)
		}
	}
	if err := client.Mail(e.from); err != nil {
		return err
	}
	for _, recipient := range e.to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(body)); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	// 服务端已接受 DATA，退出会话失败也不能重新投递同一封邮件。
	_ = client.Quit()
	log.Printf("🔔 [Email] 通知已发送 → %s", strings.Join(e.to, ","))
	return nil
}
