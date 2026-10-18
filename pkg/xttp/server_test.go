package xttp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
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
	var once sync.Once // Fiber v3.3.0 runs OnPostShutdown twice on a graceful stop
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
