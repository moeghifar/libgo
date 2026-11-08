package xttp

import (
	"io"
	"log/slog"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
)

// benchApp drives the fiber handler through an in-memory fasthttp.RequestCtx:
// no sockets, no goroutines, so ns/op is the middleware chain plus the handler.
func benchApp(b *testing.B, h fasthttp.RequestHandler) {
	b.Helper()
	var req fasthttp.Request
	req.SetRequestURI("http://example.com/")
	req.Header.SetMethod(fiber.MethodGet)
	var ctx fasthttp.RequestCtx
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		ctx.Init(&req, nil, nil)
		h(&ctx)
	}
}

func empty(c fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) }

func BenchmarkBareFiber(b *testing.B) {
	app := fiber.New()
	app.Get("/", empty)
	benchApp(b, app.Handler())
}

func BenchmarkDefaultChain(b *testing.B) {
	off := false
	cases := []struct {
		name string
		cfg  Config
	}{
		{"default", Config{RateLimit: -1}},
		{"noAccessLog", Config{RateLimit: -1, AccessLog: &off}},
		{"rateLimit", Config{}},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			cfg := tc.cfg
			cfg.Logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
			api := New(cfg)
			api.Add(Route{Method: fiber.MethodGet, Path: "/", Handler: empty})
			benchApp(b, api.App().Handler())
		})
	}
}
