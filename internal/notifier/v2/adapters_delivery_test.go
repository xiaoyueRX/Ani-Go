package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookRequiresAcknowledgement(t *testing.T) {
	for _, service := range []struct {
		name    string
		make    func(string) Notifier
		success string
	}{
		{"钉钉", func(url string) Notifier { return NewDingTalkNotifier(url, "") }, `{"errcode":0}`},
		{"企业微信", func(url string) Notifier { return NewWeComNotifier(url) }, `{"errcode":0}`},
		{"飞书", func(url string) Notifier { return NewFeishuNotifier(url) }, `{"code":0}`},
	} {
		for _, response := range []struct {
			name   string
			status int
			body   string
			ok     bool
		}{
			{"正常", 200, service.success, true},
			{"网关错误", 502, service.success, false},
			{"空响应", 200, "", false},
			{"缺少业务码", 200, "{}", false},
			{"拒绝投递", 200, `{"code":2,"errcode":2}`, false},
		} {
			t.Run(service.name+"/"+response.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(response.status)
					_, _ = w.Write([]byte(response.body))
				}))
				defer server.Close()
				err := service.make(server.URL).Send(context.Background(), "主题", "内容")
				if (err == nil) != response.ok {
					t.Fatalf("发送结果 %v", err)
				}
			})
		}
	}
}
