package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookRedirectPolicyRespectsCredentialScope(t *testing.T) {
	next := httptest.NewRequest(http.MethodGet, "https://example.org/delivery", nil)
	first := httptest.NewRequest(http.MethodPost, "https://example.org/events", nil)
	via := []*http.Request{first}
	if got := webhookDeliveryClient(false, true).CheckRedirect(next, via); got != http.ErrUseLastResponse {
		t.Fatalf("使用共享密钥时，应由调用方处理原始响应，返回 %v", got)
	}
	if err := webhookDeliveryClient(false, false).CheckRedirect(next, via); err != nil {
		t.Fatalf("普通投递仍应允许经过地址校验的重定向：%v", err)
	}
}
