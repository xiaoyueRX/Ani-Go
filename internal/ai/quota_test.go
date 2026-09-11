package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQuotaManagerLimitAndReset(t *testing.T) {
	qm := NewQuotaManager(3)

	if qm.GetDailyLimit() != 3 {
		t.Errorf("expected limit 3, got %d", qm.GetDailyLimit())
	}
	if qm.GetCurrentCount() != 0 {
		t.Errorf("expected count 0, got %d", qm.GetCurrentCount())
	}

	// 1st call
	count, ok := qm.CheckAndIncrement()
	if !ok || count != 1 {
		t.Errorf("1st call failed: ok=%v, count=%d", ok, count)
	}

	// 2nd call
	count, ok = qm.CheckAndIncrement()
	if !ok || count != 2 {
		t.Errorf("2nd call failed: ok=%v, count=%d", ok, count)
	}

	// 3rd call
	count, ok = qm.CheckAndIncrement()
	if !ok || count != 3 {
		t.Errorf("3rd call failed: ok=%v, count=%d", ok, count)
	}

	// 4th call: should be rejected
	count, ok = qm.CheckAndIncrement()
	if ok {
		t.Errorf("expected 4th call to be rejected, got ok=true")
	}
	if count != 3 {
		t.Errorf("expected count to remain 3, got %d", count)
	}
}

func TestDisambiguateTitleQuotaRejection(t *testing.T) {
	qm := GetDefaultQuotaManager()
	origLimit := qm.GetDailyLimit()
	defer qm.SetDailyLimit(origLimit)

	// Set limit to 0 / 1 and exhaust it
	qm.SetDailyLimit(1)
	qm.CheckAndIncrement() // now exhausted

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called when quota exhausted")
	}))
	defer server.Close()

	client := NewClientWithProtocol(server.URL, "dummy-key", "dummy-model", ProtocolOpenAI)
	_, err := client.DisambiguateTitle(context.Background(), "[Sakurato] Steel Ball Run - 01", "JOJO", time.Now())
	if err == nil {
		t.Fatalf("expected quota error, got nil")
	}
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("expected error to wrap ErrQuotaExceeded, got: %v", err)
	}
}
