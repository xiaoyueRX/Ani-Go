package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func fixturePanelPlugin() PluginInfo {
	return PluginInfo{ID: "cloud-example", Name: "Example", Type: "webhook", URL: "https://example.com/receive", Secret: "test-hmac", Panel: &Panel{Credential: &PanelCredential{Label: "Token", Required: true}, Actions: []PanelAction{{ID: "submit", Label: "Submit", Input: &PanelInput{Key: "text", Label: "Input"}, Result: "message"}, {ID: "list", Label: "List", Result: "tasks"}}}}
}

func TestPanelManifestHasNoExecutableFieldsOrAlternateDestinations(t *testing.T) {
	base := fixturePanelPlugin()
	for _, raw := range []string{`{"actions":[{"id":"x","label":"Submit","result":"message","url":"https://other.test"}]}`, `{"actions":[],"headers":{}}`, `{"credential":{"label":"x","default":"secret"},"actions":[]}`, `{"script":"code","actions":[]}`} {
		var p Panel
		if err := json.Unmarshal([]byte(raw), &p); err == nil {
			t.Fatal("unknown panel attribute accepted")
		}
	}
	for _, f := range []func(*PluginInfo){
		func(p *PluginInfo) { p.URL = "http://example.com/receive" }, func(p *PluginInfo) { p.URL = "https://127.0.0.1" },
		func(p *PluginInfo) { p.Type = "script" }, func(p *PluginInfo) { p.Panel.Actions[1].ID = "submit" },
		func(p *PluginInfo) { p.Panel.Actions[1].ID = "../path" }, func(p *PluginInfo) { p.Panel.Actions[0].Input.Format = "javascript" },
		func(p *PluginInfo) { p.Panel.Actions[0].Result = "html" }, func(p *PluginInfo) { p.Panel.Credential.Label = strings.Repeat("x", 300) },
	} {
		p := clonePluginInfo(base)
		f(&p)
		if err := validateWebhookPanel(p, false); err == nil {
			t.Fatalf("unsafe manifest accepted: %+v", p)
		}
	}
}
func TestPanelRunnerForwardsOnlyDeclaredActionAndPerPluginCredentials(t *testing.T) {
	m := fixturePanelPlugin()
	calls := 0
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer per-plugin-token" || r.Header.Get("X-AniGo-Secret") != "test-hmac" {
			t.Error("incorrect method or credentials")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"action":"submit","input":"example","page":1}` {
			t.Errorf("unexpected body %q", body)
		}
		io.WriteString(w, `{"message":"accepted"}`)
	}))
	defer ts.Close()
	m.URL = ts.URL
	runner := newWebhookPanelRunner(m, true)
	runner.client.Transport = ts.Client().Transport
	act := m.Panel.Actions[0]
	got, err := runner.Run(context.Background(), act, "per-plugin-token", "example", 1)
	if err != nil || got.Message != "accepted" {
		t.Fatal(got, err)
	}
	act.ID = "other"
	if _, err = runner.Run(context.Background(), act, "per-plugin-token", "example", 1); err == nil || calls != 1 {
		t.Fatal("undeclared network operation")
	}
}
func TestPanelRunnerRejectsRedirectMalformedAndOversizedOrSecretResults(t *testing.T) {
	m := fixturePanelPlugin()
	for _, body := range []string{`{"message":"test-hmac"}`, `{"message":"per-plugin-token"}`, `{"message":""}`, `{"message":"ok"}{}`, strings.Repeat("x", 2<<20)} {
		runner := newWebhookPanelRunner(m, false)
		runner.client.Transport = panelTransport(func(req *http.Request) (*http.Response, error) { return panelHTTPResponse(body), nil })
		if _, err := runner.Run(context.Background(), m.Panel.Actions[0], "per-plugin-token", "x", 1); err == nil || strings.Contains(err.Error(), "per-plugin-token") || strings.Contains(err.Error(), m.Secret) {
			t.Fatal("unsafe response accepted")
		}
	}
	runner := newWebhookPanelRunner(m, false)
	runner.client.Transport = panelTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != m.URL {
			t.Fatal("followed redirect")
		}
		resp := panelHTTPResponse("")
		resp.StatusCode = 307
		resp.Header.Set("Location", "https://different.example/")
		return resp, nil
	})
	if _, err := runner.Run(context.Background(), m.Panel.Actions[0], "per-plugin-token", "x", 1); err == nil {
		t.Fatal("redirect accepted")
	}
}
func TestPanelTaskContract(t *testing.T) {
	m := fixturePanelPlugin()
	runner := newWebhookPanelRunner(m, false)
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`{"page":1,"page_count":0,"tasks":[]}`, true}, {`{"page":1,"page_count":1,"tasks":[{"info_hash":"id","name":"job","status":"pending","progress":0}]}`, true},
		{`{"page":1,"page_count":1,"tasks":[{"info_hash":"id","name":"job","status":"completed","progress":101}]}`, false},
		{`{"page":1,"page_count":1,"tasks":[{}]}`, false}, {`{"page":1,"tasks":null}`, false},
	} {
		runner.client.Transport = panelTransport(func(*http.Request) (*http.Response, error) { return panelHTTPResponse(tc.body), nil })
		got, err := runner.Run(context.Background(), m.Panel.Actions[1], "per-plugin-token", "", 1)
		if (err == nil) != tc.ok {
			t.Fatalf("%s: %v", tc.body, err)
		}
		if tc.ok {
			b, _ := json.Marshal(got)
			if string(b) != tc.body {
				t.Fatalf("changed schema %s", b)
			}
		}
	}
}

type panelTransport func(*http.Request) (*http.Response, error)

func (f panelTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func panelHTTPResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func TestPanelSnapshotsAreLocalAndBoundToRuntime(t *testing.T) {
	setupTestDB(t)
	bus := newTestBus()
	global := core.NewContext(context.Background())
	m := NewManager(bus, global)
	defer m.StopAll()
	one := fixturePanelPlugin()
	if err := m.AddOrUpdatePlugin(one); err != nil {
		t.Fatal(err)
	}
	two := clonePluginInfo(one)
	two.ID = "other"
	if err := m.AddOrUpdatePlugin(two); err != nil {
		t.Fatal(err)
	}
	info, first, ok := m.PanelSnapshot(one.ID)
	if !ok || first == nil || info.Secret != "" {
		t.Fatal("missing runtime or leaked webhook secret")
	}
	_, second, _ := m.PanelSnapshot(two.ID)
	a, _ := first.GetService(PanelService)
	b, _ := second.GetService(PanelService)
	if a == nil || b == nil || a == b {
		t.Fatal("cross-plugin panel service")
	}
	if _, shared := global.GetService(PanelService); shared {
		t.Fatal("panel runner registered globally")
	}
	info.Panel.Actions[0].Input.Label = "Changed snapshot"
	back, _, _ := m.PanelSnapshot(one.ID)
	if back.Panel.Actions[0].Input.Label == "Changed snapshot" {
		t.Fatal("mutable snapshot")
	}
	one.Panel.Actions[0].Label = "New label"
	if err := m.AddOrUpdatePlugin(one); err != nil {
		t.Fatal(err)
	}
	_, next, _ := m.PanelSnapshot(one.ID)
	_, still, _ := m.PanelSnapshot(two.ID)
	if next == first || still != second || !errors.Is(first.Context().Err(), context.Canceled) {
		t.Fatal("wrong runtime generation for changed panel")
	}
}

func TestWebhookPanelReplacementCancelsInFlightAndKeepsRunnerLocal(t *testing.T) {
	setupTestDB(t)
	bus := newTestBus()
	global := core.NewContext(context.Background())
	manager := NewManager(bus, global)
	defer manager.StopAll()
	p := fixturePanelPlugin()
	if err := manager.AddOrUpdatePlugin(p); err != nil {
		t.Fatal(err)
	}
	_, scope, _ := manager.PanelSnapshot(p.ID)
	service, ok := LocalPanelRunner(scope)
	if !ok {
		t.Fatal("missing local panel")
	}
	global.SetService(PanelService, service)
	other := NewPluginContext("unrelated", bus, global)
	defer other.Dispose()
	if _, ok := LocalPanelRunner(other); ok {
		t.Fatal("fell back to unrelated shared runner")
	}
	runner := service.(*webhookPanelRunner)
	started := make(chan struct{})
	runner.client.Transport = panelTransport(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	done := make(chan error, 1)
	go func() {
		_, err := runner.Run(context.Background(), p.Panel.Actions[0], "per-plugin-token", "example", 1)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	p.URL = "https://another.example/receive"
	if err := manager.AddOrUpdatePlugin(p); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("old generation still active", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("old request did not cancel")
	}
	if _, ok := LocalPanelRunner(scope); ok {
		t.Fatal("disposed runner exposed")
	}
	if _, err := runner.Run(context.Background(), p.Panel.Actions[0], "per-plugin-token", "example", 1); !errors.Is(err, context.Canceled) {
		t.Fatal("stale runner accepted work")
	}
	info, next, _ := manager.PanelSnapshot(p.ID)
	if next == scope || info.URL != p.URL {
		t.Fatal("wrong endpoint paired with context")
	}
}

func TestPanelRunnerValidatesInputAndCredentialsBeforeSending(t *testing.T) {
	p := fixturePanelPlugin()
	p.Panel.Actions[0].Input.Format = "magnet"
	r := newWebhookPanelRunner(p, false)
	r.client.Transport = panelTransport(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network access"); return nil, nil })
	check := func(a PanelAction, token, input string, page int) error {
		_, err := r.Run(context.Background(), a, token, input, page)
		return err
	}
	if !errors.Is(check(p.Panel.Actions[0], "x", "not a magnet", 1), ErrPanelInput) {
		t.Fatal("invalid magnet accepted")
	}
	if !errors.Is(check(p.Panel.Actions[1], "", "", 1), ErrPanelUnauthorized) {
		t.Fatal("required credential missing")
	}
	if !errors.Is(check(p.Panel.Actions[1], "x\r\ny", "", 1), ErrPanelUnauthorized) {
		t.Fatal("malformed credential accepted")
	}
	if !errors.Is(check(p.Panel.Actions[1], "x", "", 0), ErrPanelInput) {
		t.Fatal("nonpositive page accepted")
	}
	p.Panel.Credential = nil
	r = newWebhookPanelRunner(p, false)
	r.client.Transport = panelTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("unexpected credential transmission")
		return nil, nil
	})
	if !errors.Is(check(p.Panel.Actions[1], "x", "", 1), ErrPanelUnauthorized) {
		t.Fatal("action forwarded undeclared credential")
	}
}

func TestPanelVersionCompatibilityPreservesExistingConfiguration(t *testing.T) {
	setupTestDB(t)
	m := NewManager(newTestBus())
	defer m.StopAll()
	saved := fixturePanelPlugin()
	if err := m.AddOrUpdatePlugin(saved); err != nil {
		t.Fatal(err)
	}
	_, before, _ := m.PanelSnapshot(saved.ID)
	for _, ver := range []string{"2", "0", "null", `"1"`} {
		var doc PluginInfo
		blob := `{"id":"cloud-example","name":"Replacement","type":"webhook","url":"https://other.example","panel":{"version":` + ver + `,"actions":[{"id":"x","label":"X","result":"message"}]}}`
		if err := json.Unmarshal([]byte(blob), &doc); err == nil {
			t.Fatalf("version %s was accepted", ver)
		}
	}
	info, after, _ := m.PanelSnapshot(saved.ID)
	if before != after || info.URL != saved.URL {
		t.Fatal("invalid import disturbed active plugin")
	}
	for _, raw := range []string{`{"actions":[{"id":"x","label":"X","result":"message"}]}`, `{"version":1,"actions":[{"id":"x","label":"X","result":"message"}]}`} {
		var pan Panel
		if err := json.Unmarshal([]byte(raw), &pan); err != nil {
			t.Fatal(err)
		}
		if pan.clone().Version != pan.Version {
			t.Fatal("version lost in clone")
		}
	}
	saved.Panel.Version = 2
	if err := m.AddOrUpdatePlugin(saved); err == nil {
		t.Fatal("future in-memory schema saved")
	}
}
