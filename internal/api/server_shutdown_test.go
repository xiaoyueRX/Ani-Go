package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServiceShutdownCancelsRequestAndWaitsForHandler(t *testing.T) {
	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserve.Addr().(*net.TCPAddr).Port
	reserve.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped, entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(canceled)
		<-release
	})
	srv := StartServer(ctx, "127.0.0.1", port, "test", nil, nil, nil, nil, nil, nil, nil, nil, handler, "", nil, nil, nil, ServerOptions{Stopped: stopped})
	defer srv.Close()
	client := &http.Client{Timeout: 3 * time.Second}
	go func() {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/session", port))
			if err == nil {
				resp.Body.Close()
				return
			}
			if ctx.Err() != nil {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("未运行测试请求")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("请求未收到服务取消信号")
	}
	select {
	case <-stopped:
		t.Fatal("请求尚未退出，服务已宣告结束")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("请求已退出，服务未报告关闭")
	}

}
