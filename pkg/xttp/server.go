package xttp

import (
	"context"
	"os"
	"os/signal"
	"reflect"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/healthcheck"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// Defaults applied by New when the matching Config field is zero.
const (
	DefaultReadTimeout     = 10 * time.Second
	DefaultWriteTimeout    = 30 * time.Second
	DefaultIdleTimeout     = 120 * time.Second
	DefaultBodyLimit       = 1 << 20 // 1 MiB
	DefaultRateLimit       = 100     // requests per second per client IP
	DefaultShutdownTimeout = 10 * time.Second
	MaxRequestIDLength     = 72 // two UUIDs
)

// Config describes the whole server. Every field is optional.
type Config struct {
	Info Info

	// Fiber is the base Fiber config: TrustProxy, ProxyHeader, Prefork, ...
	// ReadTimeout, WriteTimeout, IdleTimeout and BodyLimit get the Default*
	// values when zero. A negative ReadTimeout or WriteTimeout turns it off;
	// a negative IdleTimeout falls back to ReadTimeout (fasthttp's rule).
	// ErrorHandler is always replaced by xttp.ErrorHandler.
	Fiber fiber.Config

	// RateLimit is requests per second per client IP (c.IP(), so set
	// Fiber.TrustProxy behind a proxy). 0 means DefaultRateLimit, negative off.
	RateLimit int

	// CORS is enabled only when set.
	CORS *cors.Config

	Health Health

	// Listen configures Run: TLS (CertFile, CertKeyFile, CertClientFile),
	// ShutdownTimeout (DefaultShutdownTimeout when zero), ...
	Listen fiber.ListenConfig
}

// Health holds the Kubernetes probes served on GET /livez, /readyz and
// /startupz. A nil probe always reports healthy, except Ready, which also
// reports 503 once shutdown has started.
type Health struct {
	Live    func(fiber.Ctx) bool
	Ready   func(fiber.Ctx) bool
	Startup func(fiber.Ctx) bool
}

// New creates the Fiber app with xttp's defaults and starts its document.
// Mounted, in order: health probes (before every middleware, so probes are
// never rate limited or logged), then request id, slog access log, panic
// recovery, helmet, CORS (when configured) and the rate limiter. Unknown
// routes, panics and every returned error render through ErrorHandler.
func New(cfg Config) *API {
	fc := cfg.Fiber
	fc.ReadTimeout = orDefault(fc.ReadTimeout, DefaultReadTimeout)
	fc.WriteTimeout = orDefault(fc.WriteTimeout, DefaultWriteTimeout)
	fc.IdleTimeout = orDefault(fc.IdleTimeout, DefaultIdleTimeout)
	if fc.BodyLimit <= 0 {
		fc.BodyLimit = DefaultBodyLimit
	}
	fc.ErrorHandler = ErrorHandler
	app := fiber.New(fc)

	var stopping atomic.Bool
	app.Hooks().OnPreShutdown(func() error { stopping.Store(true); return nil })
	ready := cfg.Health.Ready
	app.Get(healthcheck.LivenessEndpoint, healthcheck.New(healthcheck.Config{Probe: cfg.Health.Live}))
	app.Get(healthcheck.ReadinessEndpoint, healthcheck.New(healthcheck.Config{Probe: func(c fiber.Ctx) bool {
		return !stopping.Load() && (ready == nil || ready(c))
	}}))
	app.Get(healthcheck.StartupEndpoint, healthcheck.New(healthcheck.Config{Probe: cfg.Health.Startup}))

	app.Use(dropBadRequestID, requestid.New(), accessLog, recoverer.New(), helmet.New())
	if cfg.CORS != nil {
		app.Use(cors.New(*cfg.CORS))
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = DefaultRateLimit
	}
	if cfg.RateLimit > 0 {
		app.Use(limiter.New(limiter.Config{
			Max:               cfg.RateLimit,
			Expiration:        time.Second,
			LimiterMiddleware: limiter.SlidingWindow{},
			LimitReached:      func(fiber.Ctx) error { return fiber.ErrTooManyRequests },
		}))
	}

	if cfg.Listen.ShutdownTimeout == 0 {
		cfg.Listen.ShutdownTimeout = DefaultShutdownTimeout
	}
	return &API{app: app, listen: cfg.Listen, router: app, doc: &document{
		info:    cfg.Info,
		paths:   map[string]any{},
		schemas: map[string]any{},
		names:   map[reflect.Type]string{},
	}}
}

// App returns the underlying Fiber app, e.g. for app.Test or app.Hooks():
//
//	api.App().Hooks().OnPostShutdown(func(error) error { return db.Close() })
func (a *API) App() *fiber.App { return a.app }

// Run serves on addr until ctx is done or the process gets SIGINT or SIGTERM,
// then stops accepting connections, reports not ready on /readyz and waits up
// to Listen.ShutdownTimeout for in-flight requests. Register cleanup with
// App().Hooks().OnPreShutdown / OnPostShutdown.
func (a *API) Run(ctx context.Context, addr string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	lc := a.listen
	lc.GracefulContext = ctx
	return a.app.Listen(addr, lc)
}

// dropBadRequestID removes an incoming X-Request-ID longer than
// MaxRequestIDLength or outside [A-Za-z0-9._-], so requestid generates one.
func dropBadRequestID(c fiber.Ctx) error {
	rid := c.Get(fiber.HeaderXRequestID)
	ok := len(rid) <= MaxRequestIDLength
	for i := 0; ok && i < len(rid); i++ {
		b := rid[i]
		ok = b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_' || b == '.'
	}
	if !ok {
		c.Request().Header.Del(fiber.HeaderXRequestID)
	}
	return c.Next()
}

// orDefault maps 0 to def and a negative duration to 0 (Fiber's "no timeout").
func orDefault(d, def time.Duration) time.Duration {
	switch {
	case d == 0:
		return def
	case d < 0:
		return 0
	}
	return d
}
