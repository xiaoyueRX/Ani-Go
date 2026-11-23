package notifier

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSMTPCancelClosesUnresponsiveConnection(t *testing.T) {
	server, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	host, port, _ := net.SplitHostPort(server.Addr().String())
	sender := NewEmailNotifier(host, port, "test", "", "a@example.com", []string{"b@example.com"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- sender.Send(ctx, "主题", "正文") }()
	conn, err := server.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("取消后意外发送成功")
		}
	case <-time.After(time.Second):
		t.Fatal("取消后未退出")
	}
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("底层连接未关闭: %v", err)
	}
}

func TestSMTP465StartsWithTLSHandshake(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	_ = server.SetDeadline(time.Now().Add(time.Second))
	sender := NewEmailNotifier("localhost", "465", "", "", "a@example.com", []string{"b@example.com"})
	finished := make(chan error, 1)
	go func() { finished <- sender.sendOnConnection(client, "内容", "主题") }()
	var record [5]byte
	if _, err := io.ReadFull(server, record[:]); err != nil || record[0] != 22 || record[1] != 3 {
		t.Fatalf("未收到 TLS ClientHello：%x, %v", record, err)
	}
	_ = server.Close()
	if err := <-finished; err == nil {
		t.Fatal("服务器断开后意外发送成功")
	}
}

func TestSMTP587NegotiatesSTARTTLS(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	_ = server.SetDeadline(time.Now().Add(time.Second))
	sender := NewEmailNotifier("localhost", "587", "test", "pass", "a@example.com", []string{"b@example.com"})
	finished := make(chan error, 1)
	go func() { finished <- sender.sendOnConnection(client, "内容", "主题") }()
	reader := bufio.NewReader(server)
	if _, err := io.WriteString(server, "220 localhost SMTP\r\n"); err != nil {
		t.Fatal(err)
	}
	if hello, err := reader.ReadString('\n'); err != nil || !strings.HasPrefix(hello, "EHLO ") {
		t.Fatalf("问候异常: %q %v", hello, err)
	}
	if _, err := io.WriteString(server, "250-localhost\r\n250 STARTTLS\r\n"); err != nil {
		t.Fatal(err)
	}
	if cmd, err := reader.ReadString('\n'); err != nil || cmd != "STARTTLS\r\n" {
		t.Fatalf("未请求加密: %q %v", cmd, err)
	}
	if _, err := io.WriteString(server, "220 Ready\r\n"); err != nil {
		t.Fatal(err)
	}
	if b, err := reader.ReadByte(); err != nil || b != 22 {
		t.Fatalf("未开始 TLS 握手: %v %v", b, err)
	}
	_ = server.Close()
	if err := <-finished; err == nil {
		t.Fatal("服务器断开后意外发送成功")
	}
}
