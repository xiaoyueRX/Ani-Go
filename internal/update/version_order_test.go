package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type releaseTransport string

func (tag releaseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(fmt.Sprintf("{\"tag_name\":%q}", tag)))}, nil
}

func TestReleaseVersionOrder(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            bool
	}{
		{"v0.10.0", "v0.9.0", false}, {"v0.7.0+loc", "v0.7.0", false},
		{"v0.7.0-rc.2", "v0.7.0-rc.10", true}, {"v0.7.0-rc.10", "v0.7.0", true},
		{"dev", "v0.7.0", false}, {"v0.7.0", "invalid", false},
	} {
		m := Manager{currentVersion: tc.current, client: &http.Client{Transport: releaseTransport(tc.latest)}}
		info, err := m.CheckUpdate(context.Background())
		if err != nil || info.HasUpdate != tc.want {
			t.Errorf("%s -> %s: %+v, %v", tc.current, tc.latest, info, err)
		}
	}
}

type malformedReleaseTransport struct{}

func (malformedReleaseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	body := `{"tag_name":"v1.0.0","body":[]}`
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestPartiallyDecodedReleaseIsRejected(t *testing.T) {
	m := Manager{currentVersion: "v0.7.0", client: &http.Client{Transport: malformedReleaseTransport{}}}
	info, err := m.CheckUpdate(context.Background())
	if err == nil || info.HasUpdate {
		t.Fatalf("malformed release accepted: %+v, %v", info, err)
	}
}
