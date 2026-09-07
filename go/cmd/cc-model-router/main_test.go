package main

import (
	"bufio"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type captured struct {
	mu     sync.Mutex
	method string
	path   string
	host   string
	header http.Header
	body   string
}

func (c *captured) record(r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.method = r.Method
	c.path = r.URL.Path
	c.host = r.Host
	c.header = r.Header.Clone()
	c.body = string(b)
}

func (c *captured) snapshot() captured {
	c.mu.Lock()
	defer c.mu.Unlock()
	return captured{method: c.method, path: c.path, host: c.host, header: c.header, body: c.body}
}

func newUpstream(t *testing.T, name string, c *captured) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.record(r)
		w.Header().Set("X-Upstream", name)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func newRouterServer(t *testing.T, anthropic, codex string) *httptest.Server {
	t.Helper()
	h := newHandler(config{
		anthropic:     mustURL(t, anthropic),
		codex:         mustURL(t, codex),
		codexPrefixes: splitPrefixes("gpt-"),
		codexToken:    "unused",
		logger:        log.New(io.Discard, "", 0),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, base, path, body string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

var claudeHeaders = map[string]string{
	"Authorization":     "Bearer sk-ant-oat01-secret",
	"anthropic-beta":    "oauth-2025-04-20,interleaved-thinking-2025-05-14",
	"anthropic-version": "2023-06-01",
	"Content-Type":      "application/json",
}

func TestClaudeModelPassesThroughVerbatim(t *testing.T) {
	var anth, codex captured
	anthSrv := newUpstream(t, "anthropic", &anth)
	codexSrv := newUpstream(t, "codex", &codex)
	rt := newRouterServer(t, anthSrv.URL, codexSrv.URL)

	body := `{"model":"claude-opus-5","messages":[],"stream":true}`
	resp := post(t, rt.URL, "/v1/messages?beta=true", body, claudeHeaders)

	if got := resp.Header.Get("X-Upstream"); got != "anthropic" {
		t.Fatalf("routed to %q, want anthropic", got)
	}
	got := anth.snapshot()
	if got.body != body {
		t.Errorf("body altered: %q", got.body)
	}
	if got.path != "/v1/messages" {
		t.Errorf("path = %q", got.path)
	}
	if got.host != strings.TrimPrefix(anthSrv.URL, "http://") {
		t.Errorf("Host = %q, want upstream host", got.host)
	}
	for _, k := range []string{"Authorization", "anthropic-beta", "anthropic-version"} {
		if got.header.Get(k) != claudeHeaders[k] {
			t.Errorf("%s = %q, want %q", k, got.header.Get(k), claudeHeaders[k])
		}
	}
	for _, k := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "Forwarded"} {
		if v := got.header.Get(k); v != "" {
			t.Errorf("%s = %q, want unset (verbatim pass-through)", k, v)
		}
	}
	if codex.snapshot().method != "" {
		t.Error("codex upstream received a request")
	}
}

func TestCodexModelStripsClientCredential(t *testing.T) {
	var anth, codex captured
	anthSrv := newUpstream(t, "anthropic", &anth)
	codexSrv := newUpstream(t, "codex", &codex)
	rt := newRouterServer(t, anthSrv.URL, codexSrv.URL)

	headers := map[string]string{}
	for k, v := range claudeHeaders {
		headers[k] = v
	}
	headers["x-api-key"] = "sk-ant-api03-secret"
	body := `{"model":"gpt-6-astra","messages":[{"role":"user","content":"hi"}]}`
	resp := post(t, rt.URL, "/v1/messages", body, headers)

	if got := resp.Header.Get("X-Upstream"); got != "codex" {
		t.Fatalf("routed to %q, want codex", got)
	}
	got := codex.snapshot()
	if got.body != body {
		t.Errorf("body altered: %q", got.body)
	}
	if got.header.Get("Authorization") != "Bearer unused" {
		t.Errorf("Authorization = %q, want placeholder", got.header.Get("Authorization"))
	}
	if v := got.header.Get("x-api-key"); v != "" {
		t.Errorf("x-api-key leaked: %q", v)
	}
	if got.header.Get("anthropic-beta") != claudeHeaders["anthropic-beta"] {
		t.Errorf("anthropic-beta = %q", got.header.Get("anthropic-beta"))
	}
	if anth.snapshot().method != "" {
		t.Error("anthropic upstream received a request")
	}
}

func TestRoutingRules(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   string
	}{
		{"gpt prefix is case-insensitive", http.MethodPost, "/v1/messages", `{"model":"GPT-5.6-Sol"}`, "codex"},
		{"count_tokens follows the model", http.MethodPost, "/v1/messages/count_tokens", `{"model":"gpt-6-astra"}`, "codex"},
		{"claude model", http.MethodPost, "/v1/messages", `{"model":"claude-sonnet-5"}`, "anthropic"},
		{"model with gpt in the middle", http.MethodPost, "/v1/messages", `{"model":"claude-gpt-x"}`, "anthropic"},
		{"no model field", http.MethodPost, "/v1/messages", `{"messages":[]}`, "anthropic"},
		{"non-json body", http.MethodPost, "/v1/messages", `not json`, "anthropic"},
		{"empty body", http.MethodPost, "/v1/messages", ``, "anthropic"},
		{"GET without body", http.MethodGet, "/v1/models", ``, "anthropic"},
		{"nested model key is ignored", http.MethodPost, "/v1/messages", `{"metadata":{"model":"gpt-6-astra"}}`, "anthropic"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var anth, codex captured
			anthSrv := newUpstream(t, "anthropic", &anth)
			codexSrv := newUpstream(t, "codex", &codex)
			rt := newRouterServer(t, anthSrv.URL, codexSrv.URL)

			req, err := http.NewRequest(tc.method, rt.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if got := resp.Header.Get("X-Upstream"); got != tc.want {
				t.Fatalf("routed to %q, want %q", got, tc.want)
			}
			var rec captured
			if tc.want == "anthropic" {
				rec = anth.snapshot()
			} else {
				rec = codex.snapshot()
			}
			if rec.body != tc.body {
				t.Errorf("body = %q, want %q", rec.body, tc.body)
			}
			if rec.path != tc.path {
				t.Errorf("path = %q, want %q", rec.path, tc.path)
			}
		})
	}
}

func TestStreamingIsFlushedPerChunk(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "event: message_start\ndata: {}\n\n")
		http.NewResponseController(w).Flush()
		select {
		case <-release:
		case <-time.After(5 * time.Second):
			t.Error("client never read the first chunk")
		}
		_, _ = io.WriteString(w, "event: message_stop\ndata: {}\n\n")
	}))
	t.Cleanup(upstream.Close)
	rt := newRouterServer(t, upstream.URL, upstream.URL)

	resp := post(t, rt.URL, "/v1/messages", `{"model":"gpt-6-astra","stream":true}`, claudeHeaders)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	br := bufio.NewReader(resp.Body)
	first, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read first line: %v", err)
	}
	if first != "event: message_start\n" {
		t.Fatalf("first line = %q", first)
	}
	// 最初のチャンクが上流の完了前に届いたことを確認してから上流を進める
	close(release)
	rest, err := io.ReadAll(br)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rest), "event: message_stop") {
		t.Errorf("rest = %q", rest)
	}
}

func TestUpstreamUnreachableReturns502(t *testing.T) {
	var anth captured
	anthSrv := newUpstream(t, "anthropic", &anth)
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	rt := newRouterServer(t, anthSrv.URL, dead.URL)

	resp := post(t, rt.URL, "/v1/messages", `{"model":"gpt-6-astra"}`, claudeHeaders)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `"type":"error"`) || !strings.Contains(string(b), "codex upstream unreachable") {
		t.Errorf("body = %s", b)
	}
	if anth.snapshot().method != "" {
		t.Error("anthropic upstream received a request")
	}
}

func TestHealthz(t *testing.T) {
	rt := newRouterServer(t, "https://api.anthropic.com", "http://127.0.0.1:8317")
	resp, err := http.Get(rt.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	want := "ok\ncodex=http://127.0.0.1:8317\nanthropic=https://api.anthropic.com\n"
	if resp.StatusCode != http.StatusOK || string(b) != want {
		t.Errorf("status = %d body = %q", resp.StatusCode, b)
	}
}

func TestSplitPrefixes(t *testing.T) {
	got := splitPrefixes(" gpt-, o3 ,,GPT-")
	want := []string{"gpt-", "o3", "gpt-"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %v, want %v", got, want)
	}
}
