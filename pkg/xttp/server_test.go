package xttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
)

func do(t *testing.T, app *fiber.App, req *http.Request) (*http.Response, string) {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestDefaults(t *testing.T) {
	c := New(Config{}).App().Config()
	if c.ReadTimeout != DefaultReadTimeout || c.WriteTimeout != DefaultWriteTimeout ||
		c.IdleTimeout != DefaultIdleTimeout || c.BodyLimit != DefaultBodyLimit {
		t.Errorf("defaults not applied: %+v", c)
	}
	c = New(Config{Fiber: fiber.Config{ReadTimeout: -1, WriteTimeout: 5 * time.Second, BodyLimit: 10}}).App().Config()
	if c.ReadTimeout != 0 || c.WriteTimeout != 5*time.Second || c.BodyLimit != 10 {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestHealth(t *testing.T) {
	ready := false
	app := New(Config{Health: Health{Ready: func(fiber.Ctx) bool { return ready }}}).App()
	for path, want := range map[string]int{"/livez": 200, "/startupz": 200, "/readyz": 503} {
		if resp, _ := do(t, app, httptest.NewRequest("GET", path, nil)); resp.StatusCode != want {
			t.Errorf("%s: %d, want %d", path, resp.StatusCode, want)
		}
	}
	ready = true
	if resp, _ := do(t, app, httptest.NewRequest("GET", "/readyz", nil)); resp.StatusCode != 200 {
		t.Errorf("/readyz after ready: %d", resp.StatusCode)
	}
}

func TestNotFound(t *testing.T) {
	resp, body := do(t, New(Config{}).App(), httptest.NewRequest("GET", "/nope", nil))
	if resp.StatusCode != 404 || !strings.Contains(body, `"code":"HTTP-404"`) || !strings.Contains(body, "does not exist") {
		t.Errorf("%d %s", resp.StatusCode, body)
	}
}

func TestBodyLimit(t *testing.T) {
	api := New(Config{Fiber: fiber.Config{BodyLimit: 16}})
	api.Add(Route{Method: "POST", Path: "/x", Handler: func(c fiber.Ctx) error { return c.SendStatus(204) }})
	// fasthttp rejects the body while reading it, before any handler runs.
	resp, err := api.App().Test(httptest.NewRequest("POST", "/x", strings.NewReader(strings.Repeat("a", 64))))
	if err == nil && resp.StatusCode != 413 {
		t.Errorf("status %d, want 413 or a rejected read", resp.StatusCode)
	}
}

func TestRateLimit(t *testing.T) {
	api := New(Config{RateLimit: 2})
	api.Add(Route{Method: "GET", Path: "/x", Handler: func(c fiber.Ctx) error { return c.SendStatus(204) }})
	var last *http.Response
	var body string
	for range 3 {
		last, body = do(t, api.App(), httptest.NewRequest("GET", "/x", nil))
	}
	if last.StatusCode != 429 || !strings.Contains(body, `"status":"429"`) {
		t.Errorf("%d %s", last.StatusCode, body)
	}
	// probes are mounted before the limiter
	if resp, _ := do(t, api.App(), httptest.NewRequest("GET", "/livez", nil)); resp.StatusCode != 200 {
		t.Errorf("probe rate limited: %d", resp.StatusCode)
	}
}

func TestCORS(t *testing.T) {
	preflight := func(cfg Config) string {
		api := New(cfg)
		api.Add(Route{Method: "GET", Path: "/x", Handler: func(c fiber.Ctx) error { return c.SendStatus(204) }})
		req := httptest.NewRequest("OPTIONS", "/x", nil)
		req.Header.Set("Origin", "https://a.example")
		req.Header.Set("Access-Control-Request-Method", "GET")
		resp, _ := do(t, api.App(), req)
		return resp.Header.Get("Access-Control-Allow-Origin")
	}
	if got := preflight(Config{}); got != "" {
		t.Errorf("CORS on without config: %q", got)
	}
	if got := preflight(Config{CORS: &cors.Config{AllowOrigins: []string{"https://a.example"}}}); got != "https://a.example" {
		t.Errorf("CORS off with config: %q", got)
	}
}

func TestRequestIDInput(t *testing.T) {
	app := New(Config{}).App()
	uuid := "123e4567-e89b-12d3-a456-426614174000"
	for in, keep := range map[string]bool{
		uuid:                     true,
		uuid + uuid:              true, // 72 chars
		uuid + uuid + "x":        false,
		"bad id with spaces":     false,
		strings.Repeat("a", 200): false,
	} {
		req := httptest.NewRequest("GET", "/nope", nil)
		req.Header.Set(fiber.HeaderXRequestID, in)
		resp, _ := do(t, app, req)
		got := resp.Header.Get(fiber.HeaderXRequestID)
		if (got == in) != keep || got == "" {
			t.Errorf("input len %d: got %q, keep=%v", len(in), got, keep)
		}
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	api := New(Config{Listen: fiber.ListenConfig{DisableStartupMessage: true}})
	closed := make(chan struct{})
	var once sync.Once // Fiber v3.3.0 ran OnPostShutdown twice on a graceful stop
	api.App().Hooks().OnPostShutdown(func(error) error { once.Do(func() { close(closed) }); return nil })

	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	_ = l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- api.Run(ctx, addr) }()
	for i := 0; ; i++ {
		if resp, err := http.Get("http://" + addr + "/livez"); err == nil {
			_ = resp.Body.Close()
			break
		} else if i > 100 {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("OnPostShutdown not called")
	}
}

func okRoute(api *API, method, path string) {
	api.Add(Route{Method: method, Path: path, Handler: func(c fiber.Ctx) error { return c.SendStatus(200) }})
}

func TestHelmetDefault(t *testing.T) {
	api := New(Config{})
	okRoute(api, "GET", "/x")
	resp, _ := do(t, api.App(), httptest.NewRequest("GET", "/x", nil))
	for k, v := range map[string]string{
		"X-Content-Type-Options": "nosniff", "X-Frame-Options": "SAMEORIGIN", "Referrer-Policy": "no-referrer",
		"Cross-Origin-Embedder-Policy": "require-corp", "Cache-Control": "no-store", "Server": "", "Content-Security-Policy": "",
	} {
		if got := resp.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if _, ok := resp.Header["Server"]; ok {
		t.Error("Server header present")
	}
}

func TestHelmetStrict(t *testing.T) {
	api := New(Config{HelmetPreset: HelmetStrict, Helmet: helmet.Config{XFrameOptions: "-"}, Headers: map[string]string{"X-Extra": "1"}})
	okRoute(api, "GET", "/x")
	resp, _ := do(t, api.App(), httptest.NewRequest("GET", "/x", nil))
	for k, v := range map[string]string{
		"Content-Security-Policy": StrictCSP, "Cross-Origin-Resource-Policy": "same-origin",
		"Permissions-Policy": StrictPermissionsPolicy, "Strict-Transport-Security": "", "X-Extra": "1", "X-Frame-Options": "",
	} {
		if got := resp.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	// HSTS only over TLS: app.Test has no TLS, so trust X-Forwarded-Proto
	api = New(Config{HelmetPreset: HelmetStrict, Fiber: fiber.Config{TrustProxy: true, TrustProxyConfig: fiber.TrustProxyConfig{Proxies: []string{"0.0.0.0"}}}})
	okRoute(api, "GET", "/x")
	tlsReq := httptest.NewRequest("GET", "/x", nil)
	tlsReq.Header.Set(fiber.HeaderXForwardedProto, "https")
	resp, _ = do(t, api.App(), tlsReq)
	if got := resp.Header.Get("Strict-Transport-Security"); got != "max-age=63072000; includeSubDomains; preload" {
		t.Errorf("HSTS over TLS = %q", got)
	}
	resp, _ = do(t, New(Config{HelmetPreset: HelmetStrict}).App(), httptest.NewRequest("GET", "/nope", nil))
	if resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Errorf("strict X-Frame-Options = %q", resp.Header.Get("X-Frame-Options"))
	}
}

func TestPreRoutingErrorHeaders(t *testing.T) {
	api := New(Config{HelmetPreset: HelmetStrict, Headers: map[string]string{"X-Extra": "1"}})
	okRoute(api, "POST", "/x")
	// 4 MiB body against the 1 MiB limit: fasthttp rejects it before any middleware.
	resp, err := api.App().Test(httptest.NewRequest("POST", "/x", strings.NewReader(strings.Repeat("a", 4<<20))))
	if err != nil {
		t.Skip("rejected read:", err)
	}
	if resp.StatusCode != 413 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	for k, v := range map[string]string{
		"X-Content-Type-Options": "nosniff", "Cache-Control": "no-store", "Content-Security-Policy": StrictCSP, "X-Extra": "1", "Server": "",
	} {
		if got := resp.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if resp, _ := do(t, api.App(), httptest.NewRequest("POST", "/x", nil)); resp.StatusCode != 200 {
		t.Errorf("not alive after 413: %d", resp.StatusCode)
	}
	// probes carry the extra headers too (fasthttp layer)
	if resp, _ := do(t, api.App(), httptest.NewRequest("GET", "/livez", nil)); resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("probe Cache-Control = %q", resp.Header.Get("Cache-Control"))
	}
}

func TestDocsConfig(t *testing.T) {
	status := func(t *testing.T, cfg Config, path string, hdr ...string) int {
		t.Helper()
		api := New(cfg)
		okRoute(api, "GET", "/x")
		req := httptest.NewRequest("GET", path, nil)
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		resp, _ := do(t, api.App(), req)
		return resp.StatusCode
	}
	off, on := Config{}, Config{Docs: DocsConfig{Enabled: true}}
	noPage := Config{Docs: DocsConfig{Enabled: true, Page: new(bool)}}
	guard := func(c fiber.Ctx) error {
		if c.Get("X-Key") != "k" {
			return fiber.ErrUnauthorized
		}
		return c.Next()
	}
	guarded := Config{Docs: DocsConfig{Enabled: true, Guard: []fiber.Handler{guard}}}
	tests := []struct {
		name string
		cfg  Config
		path string
		hdr  []string
		want int
	}{
		{"disabled", off, "/docs", nil, 404},
		{"page", on, "/docs", nil, 200},
		{"json", on, "/docs/openapi.json", nil, 200},
		{"yaml", on, "/docs/openapi.yaml", nil, 200},
		{"no page", noPage, "/docs", nil, 404},
		{"no page json", noPage, "/docs/openapi.json", nil, 200},
		{"guard page", guarded, "/docs", nil, 401},
		{"guard json", guarded, "/docs/openapi.json", nil, 401},
		{"guard yaml", guarded, "/docs/openapi.yaml", nil, 401},
		{"guard ok", guarded, "/docs/openapi.json", []string{"X-Key", "k"}, 200},
		{"custom path", Config{Docs: DocsConfig{Enabled: true, Path: "/ref"}}, "/ref", nil, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status(t, tt.cfg, tt.path, tt.hdr...); got != tt.want {
				t.Errorf("%s: %d, want %d", tt.path, got, tt.want)
			}
		})
	}
	t.Run("double mount panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		New(on).Docs("/docs")
	})
}

func TestDocsPageCSP(t *testing.T) {
	api := New(Config{HelmetPreset: HelmetStrict, Docs: DocsConfig{Enabled: true}})
	resp, _ := do(t, api.App(), httptest.NewRequest("GET", "/docs", nil))
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src "+scalarJS) || strings.Contains(csp, "script-src 'unsafe-inline'") {
		t.Errorf("docs CSP = %q", csp)
	}
	resp, _ = do(t, api.App(), httptest.NewRequest("GET", "/docs/openapi.json", nil))
	if resp.Header.Get("Content-Security-Policy") != StrictCSP {
		t.Errorf("spec file CSP relaxed: %q", resp.Header.Get("Content-Security-Policy"))
	}
}

func TestProfiling(t *testing.T) {
	guard := func(c fiber.Ctx) error {
		if c.Get("X-Key") != "k" {
			return fiber.ErrUnauthorized
		}
		return c.Next()
	}
	if resp, _ := do(t, New(Config{}).App(), httptest.NewRequest("GET", "/debug/pprof/", nil)); resp.StatusCode != 404 {
		t.Errorf("disabled: %d", resp.StatusCode)
	}
	app := New(Config{Profiling: ProfilingConfig{Enabled: true, Guard: []fiber.Handler{guard}}}).App()
	if resp, _ := do(t, app, httptest.NewRequest("GET", "/debug/pprof/", nil)); resp.StatusCode != 401 {
		t.Errorf("no credentials: %d", resp.StatusCode)
	}
	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req.Header.Set("X-Key", "k")
	if resp, _ := do(t, app, req); resp.StatusCode != 200 {
		t.Errorf("with credentials: %d", resp.StatusCode)
	}
	app = New(Config{Profiling: ProfilingConfig{Enabled: true, AllowUnguarded: true, Prefix: "/int"}}).App()
	if resp, _ := do(t, app, httptest.NewRequest("GET", "/int/debug/pprof/", nil)); resp.StatusCode != 200 {
		t.Errorf("unguarded with prefix: %d", resp.StatusCode)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic: profiling without guard")
		}
	}()
	New(Config{Profiling: ProfilingConfig{Enabled: true}})
}

func TestListenerNetwork(t *testing.T) {
	if got := New(Config{}).listen.ListenerNetwork; got != fiber.NetworkTCP {
		t.Errorf("default %q", got)
	}
	if got := New(Config{Listen: fiber.ListenConfig{ListenerNetwork: fiber.NetworkTCP4}}).listen.ListenerNetwork; got != fiber.NetworkTCP4 {
		t.Errorf("explicit %q", got)
	}
}

func TestLogger(t *testing.T) {
	var buf, def bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&def, nil)))
	defer slog.SetDefault(old)

	api := New(Config{Logger: slog.New(slog.NewTextHandler(&buf, nil))})
	api.Add(Route{Method: "GET", Path: "/boom", Handler: func(fiber.Ctx) error { return errors.New("boom") }})
	okRoute(api, "GET", "/x")
	do(t, api.App(), httptest.NewRequest("GET", "/x", nil))
	do(t, api.App(), httptest.NewRequest("GET", "/boom", nil))
	if !strings.Contains(buf.String(), "http request") || !strings.Contains(buf.String(), "http error") {
		t.Errorf("logger got:\n%s", buf.String())
	}
	if def.Len() != 0 {
		t.Errorf("default logger got:\n%s", def.String())
	}

	buf.Reset()
	off := false
	api = New(Config{Logger: slog.New(slog.NewTextHandler(&buf, nil)), AccessLog: &off})
	api.Add(Route{Method: "GET", Path: "/boom", Handler: func(fiber.Ctx) error { return errors.New("boom") }})
	okRoute(api, "GET", "/x")
	do(t, api.App(), httptest.NewRequest("GET", "/x", nil))
	do(t, api.App(), httptest.NewRequest("GET", "/boom", nil))
	if strings.Contains(buf.String(), "http request") || !strings.Contains(buf.String(), "http error") {
		t.Errorf("AccessLog off got:\n%s", buf.String())
	}
}

func TestRateLimitHeaders(t *testing.T) {
	api := New(Config{})
	okRoute(api, "GET", "/x")
	if resp, _ := do(t, api.App(), httptest.NewRequest("GET", "/x", nil)); resp.Header.Get("X-RateLimit-Limit") != "" {
		t.Error("X-RateLimit-Limit leaked by default")
	}
	on := true
	api = New(Config{RateLimitHeaders: &on})
	okRoute(api, "GET", "/x")
	if resp, _ := do(t, api.App(), httptest.NewRequest("GET", "/x", nil)); resp.Header.Get("X-RateLimit-Limit") == "" {
		t.Error("X-RateLimit-Limit missing when enabled")
	}
}

func TestStrictContentType(t *testing.T) {
	api := New(Config{StrictContentType: true})
	okRoute(api, "POST", "/x")
	okRoute(api, "GET", "/g")
	post := func(ct, body string) (*http.Response, string) {
		req := httptest.NewRequest("POST", "/x", strings.NewReader(body))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		return do(t, api.App(), req)
	}
	if resp, body := post("text/plain", `{"a":1}`); resp.StatusCode != 415 || !strings.Contains(body, `"status":"415"`) {
		t.Errorf("text/plain: %d %s", resp.StatusCode, body)
	}
	if resp, _ := post("application/json; charset=utf-8", `{"a":1}`); resp.StatusCode != 200 {
		t.Errorf("json with charset: %d", resp.StatusCode)
	}
	if resp, _ := post("", ""); resp.StatusCode != 200 {
		t.Errorf("bodiless POST: %d", resp.StatusCode)
	}
	if resp, _ := do(t, api.App(), httptest.NewRequest("DELETE", "/g", nil)); resp.StatusCode != 405 {
		t.Errorf("bodiless DELETE on GET route: %d", resp.StatusCode)
	}
	api = New(Config{})
	okRoute(api, "POST", "/x")
	if resp, _ := post("text/plain", `{"a":1}`); resp.StatusCode != 200 {
		t.Errorf("default accepts any content type: %d", resp.StatusCode)
	}
}
