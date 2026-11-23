package cloud115

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

const fixtureHash = "0123456789abcdef0123456789abcdef01234567"
const magnet = "magnet:?xt=urn:btih:" + fixtureHash + "&dn=Example%20Title&tr=https%3A%2F%2Fexample.com%2Fa%3Fx%3D1%26y%3D2"
const token = "private-test-token"
const accepted = `{"state":true,"code":0,"data":[{"state":true,"code":0,"info_hash":"` + fixtureHash + `"}]}`

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func reply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestSubmitMagnetUsesOpenAPIAndValidatesReceipt(t *testing.T) {
	client := NewClient()
	client.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.String() != "https://proapi.115.com/open/offline/add_task_urls" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("unexpected request to %s", r.URL)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatal("unexpected media type")
		}
		if err := r.ParseForm(); err != nil || len(r.PostForm["urls"]) != 1 || r.PostForm.Get("urls") != magnet || len(r.PostForm) != 1 {
			t.Fatal("incorrect URL form encoding")
		}
		return reply(200, accepted), nil
	})
	if err := client.Add(context.Background(), token, magnet); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsMalformedAndMultilineMagnets(t *testing.T) {
	client := NewClient()
	client.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network access"); return nil, nil })
	for _, s := range []string{"", "https://example.com/private", "magnet:?xt=urn:btih:bad", magnet + "\n" + magnet, "magnet://host/?xt=urn:btih:" + fixtureHash, magnet + "%xy", "magnet:?dn=example"} {
		if err := client.Add(context.Background(), token, s); !errors.Is(err, ErrInvalidMagnet) {
			t.Errorf("expected invalid magnet, got %v", err)
		}
	}
	for _, s := range []string{magnet, "magnet:?xt=urn:btih:ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", "magnet:?xt=urn:btmh:1220" + strings.Repeat("ab", 32)} {
		if err := ValidateMagnet(s); err != nil {
			t.Errorf("valid magnet rejected: %v", err)
		}
	}
	if err := client.Add(context.Background(), "", magnet); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
}

func TestFailureAndIncompleteResponsesCannotReportSubmission(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"expired", 200, `{"state":false,"code":40140125,"message":"` + token + `"}`, ErrUnauthorized},
		{"alternate_auth_error", 200, `{"state":0,"errno":40140124,"error":"` + token + `"}`, ErrUnauthorized},
		{"http_unauthorized", 401, token, ErrUnauthorized}, {"http_limited", 429, token, ErrRateLimited},
		{"refusal", 200, `{"state":false,"code":10008,"message":"` + token + `"}`, nil},
		{"nested_refusal", 200, `{"state":true,"code":0,"data":[{"state":false,"code":10008,"message":"` + token + `"}]}`, nil},
		{"no_task", 200, `{"state":true,"code":0,"data":[]}`, nil},
		{"missing_item_code", 200, `{"state":true,"code":0,"data":[{"state":true,"info_hash":"hash"}]}`, nil},
		{"missing_hash", 200, `{"state":true,"code":0,"data":[{"state":true,"code":0}]}`, nil},
		{"missing_outer_code", 200, `{"state":true,"data":[{"state":true,"code":0,"info_hash":"hash"}]}`, nil},
		{"malformed", 200, accepted + "garbage", nil}, {"http_failure", 500, token, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient()
			c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return reply(tc.status, tc.body), nil })
			err := c.Add(context.Background(), token, magnet)
			if err == nil || tc.want != nil && !errors.Is(err, tc.want) || strings.Contains(err.Error(), token) || strings.Contains(err.Error(), magnet) {
				t.Fatalf("unexpected error classification: %v", err)
			}
		})
	}
}

func TestListNormalizesCloudTasks(t *testing.T) {
	c := NewClient()
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.String() != "https://proapi.115.com/open/offline/get_task_list?page=2" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatal("unexpected list request")
		}
		return reply(200, `{"state":true,"code":0,"data":{"page":2,"page_count":3,"tasks":[{"info_hash":"a","name":"queued","percentDone":-1,"status":0},{"info_hash":"b","name":"active","percentDone":23,"status":1},{"info_hash":"c","name":"finished","percentDone":103,"status":2},{"info_hash":"d","name":"error","percentDone":5,"status":-1},{"info_hash":"e","name":"future","percentDone":5,"status":12}]}}`), nil
	})
	p, err := c.List(context.Background(), token, 2)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(p)
	want := `{"page":2,"page_count":3,"tasks":[{"info_hash":"a","name":"queued","status":"pending","progress":0},{"info_hash":"b","name":"active","status":"downloading","progress":23},{"info_hash":"c","name":"finished","status":"completed","progress":100},{"info_hash":"d","name":"error","status":"failed","progress":5},{"info_hash":"e","name":"future","status":"unknown","progress":5}]}`
	if string(got) != want {
		t.Fatalf("unexpected list: %s", got)
	}
}

func TestListDistinguishesEmptyPageFromInvalidResponse(t *testing.T) {
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`{"state":true,"code":0,"data":{"page":1,"page_count":0,"tasks":[]}}`, true},
		{`{"state":true,"code":0,"data":{}}`, false},
		{`{"state":true,"code":0,"data":null}`, false},
		{`{"state":true,"code":0,"data":{"page":1,"page_count":0,"count":0,"tasks":null}}`, true},
		{`{"state":true,"code":0,"data":{"page":1,"page_count":1,"count":2,"tasks":null}}`, false},
	} {
		c := NewClient()
		c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return reply(200, tc.body), nil })
		p, err := c.List(context.Background(), token, 1)
		if (err == nil) != tc.ok || tc.ok && (len(p.Tasks) != 0 || p.Tasks == nil) {
			t.Errorf("unexpected empty page: %v, %v", p, err)
		}
	}
}

func TestRedirectAndTransportErrorsPreserveTokenBoundary(t *testing.T) {
	c := NewClient()
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "proapi.115.com" {
			t.Fatal("redirect followed")
		}
		resp := reply(307, "")
		resp.Header.Set("Location", "https://elsewhere.example/")
		return resp, nil
	})
	if err := c.Add(context.Background(), token, magnet); err == nil {
		t.Fatal("redirect accepted")
	}
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New(token + " " + magnet) })
	if err := c.Add(context.Background(), token, magnet); err == nil || strings.Contains(err.Error(), token) || strings.Contains(err.Error(), magnet) {
		t.Fatal("transport details leaked")
	}
}

func TestSubmissionReceiptMatchesBTIHAcrossEncodings(t *testing.T) {
	bytes, _ := hex.DecodeString(fixtureHash)
	base32hash := base32.StdEncoding.EncodeToString(bytes)
	const v2 = "urn:btmh:1220"
	hashv2 := strings.Repeat("ab", 32)
	for _, tc := range []struct {
		name, link, returned string
		ok                   bool
	}{
		{"hex_case", "magnet:?xt=urn:btih:" + fixtureHash, strings.ToUpper(fixtureHash), true},
		{"base32", "magnet:?xt=urn:btih:" + base32hash, fixtureHash, true},
		{"different_task", magnet, strings.Repeat("ab", 20), false},
		{"hybrid_checks_v1", "magnet:?xt=" + v2 + hashv2 + "&xt=urn:btih:" + fixtureHash, strings.Repeat("ab", 20), false},
		{"v2_task_sha1_needs_echo", "magnet:?xt=" + v2 + hashv2, fixtureHash, false},
		{"v2_missing_task_sha1", "magnet:?xt=" + v2 + hashv2, "not-a-sha1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient()
			c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				return reply(200, strings.Replace(accepted, fixtureHash, tc.returned, 1)), nil
			})
			err := c.Add(context.Background(), token, tc.link)
			if (err == nil) != tc.ok {
				t.Fatalf("incorrect receipt outcome: %v", err)
			}
		})
	}
}

func TestTaskNumericVariantsPreserveOtherTasksOnPage(t *testing.T) {
	c := NewClient()
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		return reply(200, `{"state":true,"code":0,"data":{"page":1,"page_count":1,"tasks":[{"info_hash":"a","status":"1","percentDone":"30.6"},{"info_hash":"b","status":2,"percentDone":99.9},{"info_hash":"c","status":"new","percentDone":null},{"info_hash":"d","name":"missing status"},{"info_hash":"e","name":{},"status":2,"percentDone":100}]}}`), nil
	})
	p, err := c.List(context.Background(), token, 1)
	if err != nil || len(p.Tasks) != 5 {
		t.Fatalf("page lost due to one task: %+v, %v", p, err)
	}
	if p.Tasks[0].Status != "downloading" || p.Tasks[0].Progress != 30 || p.Tasks[1].Status != "completed" || p.Tasks[1].Progress != 99 || p.Tasks[2].Status != "unknown" || p.Tasks[3].Status != "unknown" || p.Tasks[4].Progress != 100 {
		t.Fatalf("wrong normalization: %+v", p.Tasks)
	}
}

type closeTracker struct {
	transportFunc
	closed bool
}

func (c *closeTracker) CloseIdleConnections() { c.closed = true }

func TestClientOwnsAndClosesItsIdleConnections(t *testing.T) {
	first, second := NewClient(), NewClient()
	one, ok := first.http.Transport.(*http.Transport)
	if !ok || one == http.DefaultTransport || one == second.http.Transport || one.Proxy == nil {
		t.Fatal("expected a private clone of the standard transport")
	}
	tracker := &closeTracker{}
	first.http.Transport = tracker
	first.CloseIdleConnections()
	if !tracker.closed {
		t.Fatal("plugin disposal must close idle connections")
	}
}

func TestSubmissionRequiresEchoedV2IdentityAndRejectsConflictingTopics(t *testing.T) {
	v2 := "urn:btmh:1220" + strings.Repeat("a1", 32)
	otherV2 := "urn:btmh:1220" + strings.Repeat("b2", 32)
	v1 := "urn:btih:" + fixtureHash
	otherV1 := "urn:btih:" + strings.Repeat("ab", 20)
	link := func(topics ...string) string { return "magnet:?xt=" + strings.Join(topics, "&xt=") }
	for _, tc := range []struct {
		name, requested, echoed string
		ok                      bool
	}{
		{"v2_ignores_metadata_and_case", link(v2) + "&dn=original", link(strings.ToUpper(v2)) + "&dn=renamed", true},
		{"v2_wrong_echo", link(v2), link(otherV2), false},
		{"v2_missing_echo", link(v2), "", false},
		{"v2_echo_lacks_v2", link(v2), link(v1), false},
		{"echoed_v1_conflicts_with_receipt_hash", link(v2), link(v2, otherV1), false},
		{"v1_accepts_matching_echo", link(v1), link(v1), true},
		{"v1_accepts_hash_only", link(v1), "", true},
		{"v1_receipt_has_unrelated_url", link(v1), link(otherV1), false},
		{"hybrid_matching", link(v1, v2), link(v2, v1), true},
		{"hybrid_known_v1", link(v1, v2), link(v1), true},
		{"hybrid_hash_only", link(v1, v2), "", true},
		{"hybrid_rejects_conflicting_v2", link(v1, v2), link(v1, otherV2), false},
		{"duplicate_conflicting_v2", link(v2), link(v2, otherV2), false},
		{"duplicate_conflicting_v1", link(v1), link(v1, otherV1), false},
		{"duplicate_identical_topics", link(v2), link(v2, v2), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient()
			c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
				body := accepted
				if tc.echoed != "" {
					b, _ := json.Marshal(tc.echoed)
					body = strings.Replace(body, `"info_hash":`, `"url":`+string(b)+`,"info_hash":`, 1)
				}
				return reply(200, body), nil
			})
			err := c.Add(context.Background(), token, tc.requested)
			if (err == nil) != tc.ok {
				t.Fatalf("unexpected acknowledgement: %v", err)
			}
		})
	}
}

func TestTaskListRejectsServiceCredentialInDisplayFields(t *testing.T) {
	c := NewClient()
	for _, field := range []string{"info_hash", "name"} {
		t.Run(field, func(t *testing.T) {
			row := map[string]string{"info_hash": "fixture", "name": "example"}
			row[field] = "prefix-" + token + "-suffix"
			body, _ := json.Marshal(row)
			c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
				return reply(200, `{"state":true,"code":0,"data":{"page":1,"page_count":1,"tasks":[`+string(body)+`]}}`), nil
			})
			if p, err := c.List(t.Context(), token, 1); err == nil || strings.Contains(err.Error(), token) || len(p.Tasks) > 0 {
				t.Fatal("远端敏感字段进入了任务结果")
			}
		})
	}
}
