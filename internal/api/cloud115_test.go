package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/cloud115"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

func TestCloud115PluginConfigurationAndLifecycle(t *testing.T) {
	setupTestDB(t)
	const envKey = "CLOUD115_ACCESS_TOKEN"
	t.Setenv(envKey, "")
	mgr := plugin.NewManager(&dummyEventBus{})
	mgr.Load()
	t.Cleanup(mgr.StopAll)
	mux := http.NewServeMux()
	(&Server{pluginManager: mgr}).registerRoutes(mux)
	call := func(method, path, body string, want int) string {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s %s got %d, want %d: %s", method, path, w.Code, want, w.Body)
		}
		if strings.Contains(w.Body.String(), "sample-secret-115") {
			t.Fatalf("%s %s disclosed the test credential", method, path)
		}
		return w.Body.String()
	}
	const root = "/api/plugins/cloud-115/"
	call("GET", root+"config", "", 409)
	if err := mgr.TogglePlugin("cloud-115", true); err != nil {
		t.Fatal(err)
	}
	checkSource := func(source string, configured bool) {
		t.Helper()
		var result struct {
			Source     string
			Configured bool
		}
		if err := json.Unmarshal([]byte(call("GET", root+"config", "", 200)), &result); err != nil || result.Source != source || result.Configured != configured {
			t.Fatalf("config = %+v, err %v, want %s/%t", result, err, source, configured)
		}
	}
	checkSource("none", false)
	call("HEAD", root+"config", `{"access_token":"must-not-be-stored"}`, 200)
	checkSource("none", false)
	call("GET", root+"tasks", "", 409)
	call("PUT", root+"config", `{"access_token":"sample-secret-115"}`, 200)
	checkSource("stored", true)
	call("GET", "/api/settings", "", 200)
	call("POST", root+"tasks", `{"magnet":"not a magnet"}`, 400)
	call("GET", root+"tasks?page=0", "", 400)
	t.Setenv(envKey, "sample-secret-115-env")
	checkSource("environment", true)
	call("PUT", root+"config", `{"access_token":"other"}`, 409)
	call("DELETE", root+"config", "", 409)
	t.Setenv(envKey, "")
	call("DELETE", root+"config", "", 200)
	checkSource("none", false)
	if err := mgr.TogglePlugin("cloud-115", false); err != nil {
		t.Fatal(err)
	}
	call("GET", root+"tasks", "", 409)
}

type pendingCloudRequest struct{ started chan struct{} }

func (p pendingCloudRequest) Add(ctx context.Context, _, _ string) error {
	close(p.started)
	<-ctx.Done()
	return ctx.Err()
}

func (p pendingCloudRequest) List(context.Context, string, int) (cloud115.TaskPage, error) {
	return cloud115.TaskPage{}, nil
}

func TestDisablingCloud115CancelsAnInFlightSubmission(t *testing.T) {
	setupTestDB(t)
	t.Setenv("CLOUD115_ACCESS_TOKEN", "fixture-token")
	mgr := plugin.NewManager(&dummyEventBus{})
	t.Cleanup(mgr.StopAll)
	if err := mgr.Enable(plugin.Cloud115ID); err != nil {
		t.Fatal(err)
	}
	client := pendingCloudRequest{started: make(chan struct{})}
	mgr.GetPluginContext(plugin.Cloud115ID).SetService(plugin.Cloud115Service, client)
	mux := http.NewServeMux()
	(&Server{pluginManager: mgr}).registerRoutes(mux)
	result := make(chan int, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/plugins/cloud-115/tasks", strings.NewReader(`{"magnet":"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"}`))
		mux.ServeHTTP(w, req.WithContext(ctx))
		result <- w.Code
	}()
	select {
	case <-client.started:
	case <-time.After(3 * time.Second):
		t.Fatal("submission did not reach the active plugin")
	}
	if err := mgr.Disable(plugin.Cloud115ID); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-result:
		if code != http.StatusConflict {
			t.Fatalf("canceled submission returned %d", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("plugin disable did not cancel the provider request")
	}
}
