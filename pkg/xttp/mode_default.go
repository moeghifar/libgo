package xttp

import (
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/healthcheck"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// ModeDefault registers the probes and the middleware chain: robust and secure
// by design. Use it unless you have measured a reason not to.
func ModeDefault(a *API, app *fiber.App, cfg *Config) {
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
	if a.accessLog || a.onResponse != nil {
		app.Use(a.observe)
	}
	app.Use(recoverer.New(), a.headers.middleware)
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
			DisableHeaders:    cfg.RateLimitHeaders == nil || !*cfg.RateLimitHeaders,
		}))
	}
	if cfg.StrictContentType {
		app.Use(strictContentType)
	}
}
