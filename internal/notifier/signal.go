package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type SignalNotifier struct {
	APIURL     string
	Sender     string
	Recipients []string
}

func NewSignalNotifier() *SignalNotifier {
	recipientsStr := os.Getenv("SIGNAL_RECIPIENTS")
	var recipients []string
	if recipientsStr != "" {
		for _, r := range strings.Split(recipientsStr, ",") {
			recipients = append(recipients, strings.TrimSpace(r))
		}
	}

	return &SignalNotifier{
		APIURL:     os.Getenv("SIGNAL_API_URL"),
		Sender:     os.Getenv("SIGNAL_SENDER"),
		Recipients: recipients,
	}
}

func (n *SignalNotifier) Name() string {
	return "Signal"
}

func (n *SignalNotifier) Send(ctx context.Context, title, message string) error {
	if n.APIURL == "" || n.Sender == "" || len(n.Recipients) == 0 {
		return fmt.Errorf("signal credentials not configured")
	}

	url := fmt.Sprintf("%s/v2/send", strings.TrimRight(n.APIURL, "/"))

	payload := map[string]interface{}{
		"message":    fmt.Sprintf("【%s】\n%s", title, message),
		"number":     n.Sender,
		"recipients": n.Recipients,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("signal api returned error status: %d", resp.StatusCode)
	}

	return nil
}
