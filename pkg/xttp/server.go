package xttp

import (
	"context"
	"log/slog"
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
	"github.com/gofiber/fiber/v3/middleware/pprof"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/valyala/fasthttp"
)

// Defaults applied by New when the matching Config field is zero.
const (
	DefaultReadTimeout     = 10 * time.Second
	DefaultWriteTimeout    = 30 * time.Second
	DefaultIdleTimeout     = 120 * time.Second
	DefaultBodyLimit       = 1 << 20 // 1 MiB
	DefaultRateLimit       = 100     // requests per second per client IP
	DefaultShutdownTimeout = 10 * time.Second
	DefaultDocsPath        = "/docs"
	MaxRequestIDLength     = 72 // two UUIDs
)

// Config describes the whole server. Every field is optional: zero values keep
// the defaults, security features are on by default and debug features off.
type Config struct {
	Info Info

	// Fiber is the base Fiber config: TrustProxy, ProxyHeader, Prefork, ...
	// ReadTimeout, WriteTimeout, IdleTimeout and BodyLimit get the Default*
	// values when zero. A negative ReadTimeout or WriteTimeout turns it off;
	// a negative IdleTimeout falls back to ReadTimeout (fasthttp's rule).
	// ErrorHandler is always replaced by (*API).ErrorHandler.
	Fiber fiber.Config

	// RateLimit is requests per second per client IP (c.IP(), so set
	// Fiber.TrustProxy behind a proxy). 0 means DefaultRateLimit, negative off.
	RateLimit int
	// RateLimitHeaders sends X-RateLimit-Limit/Remaining/Reset. Default false:
	// they tell a client the exact budget.
	RateLimitHeaders *bool

	// StrictContentType answers 415 to any request with a body whose media type
	// is not application/json (parameters allowed), before any handler.
	// Bodiless requests pass. Default false.
	StrictContentType bool

	// CORS is enabled only when set.
	CORS *cors.Config

	Health Health

	// Helmet overrides the preset's security headers field by field; "-" omits
	// that header. Helmet.Next skips the headers for a request. There is no
	// way to turn the headers off altogether.
	Helmet helmet.Config
	// HelmetPreset is HelmetDefault (helmet.ConfigDefault + Cache-Control:
	// no-store) or HelmetStrict.
	HelmetPreset HelmetPreset
	// HideServerHeader removes the Server banner, also on responses fasthttp
	// writes before routing. Default true; false keeps Fiber.ServerHeader.
	HideServerHeader *bool
	// Headers are extra static headers on every response, including probes and
	// requests rejected before routing (413, 431). "-" removes a default, e.g.
	// {"Cache-Control": "-"}.
	Headers map[string]string

	// Docs serves the OpenAPI document and the Scalar page. Off by default.
	Docs DocsConfig
	// Profiling mounts net/http/pprof behind a guard. Off by default.
	Profiling ProfilingConfig

	// Logger receives the access log and server errors. Nil means slog.Default().
	Logger *slog.Logger
	// AccessLog writes one line per request. Default true. Errors >= 500 are
	// logged either way.
	AccessLog *bool

	// DefaultErrors are statuses documented on every route as ErrorDocument
	// unless the route sets them, e.g. []int{400, 404, 500}.
	DefaultErrors []int

	// Listen configures Run: TLS (CertFile, CertKeyFile, CertClientFile),
	// ShutdownTimeout (DefaultShutdownTimeout when zero), ListenerNetwork
	// (dual-stack "tcp" when empty), ...
	Listen fiber.ListenConfig
}

// DocsConfig serves the document under Path: the Scalar page at Path, and
// Path+"/openapi.yaml", Path+"/openapi.json".
type DocsConfig struct {
	Enabled bool            // default false
	Path    string          // default DefaultDocsPath
	Page    *bool           // HTML viewer, which loads Scalar from a CDN. Default true; false serves only the spec files
	Guard   []fiber.Handler // run before every docs route: auth, IP allowlist
}

// ProfilingConfig mounts Fiber's pprof middleware at Prefix+"/debug/pprof/*".
// pprof exposes heap contents and goroutine stacks, so New panics when Enabled
// with no Guard unless AllowUnguarded is set explicitly (local development).
type ProfilingConfig struct {
	Enabled        bool
	Prefix         string          // URL prefix before "/debug/pprof", e.g. "/internal"
	Guard          []fiber.Handler // run before pprof
	AllowUnguarded bool
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
// recovery, security headers, CORS (when configured), pprof (when enabled),
// the rate limiter, the content-type check (when enabled) and the docs routes
// (when enabled). Unknown routes, panics and every returned error render
// through (*API).ErrorHandler.
func New(cfg Config) *API {
	if cfg.Profiling.Enabled && len(cfg.Profiling.Guard) == 0 && !cfg.Profiling.AllowUnguarded {
		panic("xttp: Profiling.Enabled needs a Guard (or AllowUnguarded for local development)")
	}
	a := &API{
		log:     cfg.Logger,
		headers: newHeaderSet(helmetFor(cfg.HelmetPreset, cfg.Helmet), cfg.Headers),
		doc: &document{
			info:          cfg.Info,
			paths:         map[string]any{},
			schemas:       map[string]any{},
			names:         map[reflect.Type]string{},
			defaultErrors: cfg.DefaultErrors,
			mounted:       map[string]bool{},
		},
	}
	if a.log == nil {
		a.log = slog.Default()
	}

	fc := cfg.Fiber
	fc.ReadTimeout = orDefault(fc.ReadTimeout, DefaultReadTimeout)
	fc.WriteTimeout = orDefault(fc.WriteTimeout, DefaultWriteTimeout)
	fc.IdleTimeout = orDefault(fc.IdleTimeout, DefaultIdleTimeout)
	if fc.BodyLimit <= 0 {
		fc.BodyLimit = DefaultBodyLimit
	}
	fc.ErrorHandler = a.ErrorHandler
	app := fiber.New(fc)
	a.app, a.router = app, app
	a.harden(app.Server(), on(cfg.HideServerHeader))

	var stopping atomic.Bool
	app.Hooks().OnPreShutdown(func() error { stopping.Store(true); return nil })
	ready := cfg.Health.Ready
	app.Get(healthcheck.LivenessEndpoint, healthcheck.New(healthcheck.Config{Probe: cfg.Health.Live}))
	app.Get(healthcheck.ReadinessEndpoint, healthcheck.New(healthcheck.Config{Probe: func(c fiber.Ctx) bool {
		return !stopping.Load() && (ready == nil || ready(c))
	}}))
	app.Get(healthcheck.StartupEndpoint, healthcheck.New(healthcheck.Config{Probe: cfg.Health.Startup}))

	app.Use(dropBadRequestID, requestid.New())
	if on(cfg.AccessLog) {
		app.Use(a.accessLog)
	}
	app.Use(recoverer.New(), a.headers.middleware)
	if cfg.CORS != nil {
		app.Use(cors.New(*cfg.CORS))
	}
	if cfg.Profiling.Enabled {
		chain := append([]any{cfg.Profiling.Prefix + "/debug/pprof"}, handlers(cfg.Profiling.Guard)...)
		app.Use(append(chain, pprof.New(pprof.Config{Prefix: cfg.Profiling.Prefix}))...)
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
			DisableHeaders:    cfg.RateLimitHeaders == nil || !*cfg.RateLimitHeaders,
		}))
	}
	if cfg.StrictContentType {
		app.Use(strictContentType)
	}
	if cfg.Docs.Enabled {
		path := cfg.Docs.Path
		if path == "" {
			path = DefaultDocsPath
		}
		a.docs(path, on(cfg.Docs.Page), cfg.Docs.Guard)
	}

	if cfg.Listen.ShutdownTimeout == 0 {
		cfg.Listen.ShutdownTimeout = DefaultShutdownTimeout
	}
	if cfg.Listen.ListenerNetwork == "" {
		cfg.Listen.ListenerNetwork = fiber.NetworkTCP // Fiber's default is tcp4: no IPv6
	}
	a.listen = cfg.Listen
	return a
}

// harden works at the fasthttp layer so it covers every response, including
// probes and requests rejected before routing: no Server banner, and
// Config.Headers (with Cache-Control: no-store) wherever a handler did not
// set them.
func (a *API) harden(srv *fasthttp.Server, hideServer bool) {
	if hideServer {
		srv.Name, srv.NoDefaultServerHeader = "", true
	}
	next, nextErr := srv.Handler, srv.ErrorHandler
	add := func(ctx *fasthttp.RequestCtx) {
		for _, h := range a.headers.extra {
			if len(ctx.Response.Header.Peek(h[0])) == 0 {
				ctx.Response.Header.Set(h[0], h[1])
			}
		}
	}
	srv.Handler = func(ctx *fasthttp.RequestCtx) { next(ctx); add(ctx) }
	srv.ErrorHandler = func(ctx *fasthttp.RequestCtx, err error) { nextErr(ctx, err); add(ctx) }
}

// on reads a *bool whose nil means true.
func on(b *bool) bool { return b == nil || *b }

func handlers(hs []fiber.Handler) []any {
	out := make([]any, len(hs))
	for i, h := range hs {
		out[i] = h
	}
	return out
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
