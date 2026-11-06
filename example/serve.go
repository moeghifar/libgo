package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/moeghifar/libgo/example/internal/user"
	"github.com/moeghifar/libgo/pkg/foreverwork"
	"github.com/moeghifar/libgo/pkg/xttp"
)

// serve runs the API and the worker pool until ctx ends, then drains the API
// first so no request submits to a stopped pool.
func serve(ctx context.Context, cfg ServeConfig) error {
	if err := cfg.setup(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wp := foreverwork.New()
	welcome := foreverwork.Register(wp,
		foreverwork.Config{Name: "welcome", Workers: 2, QueueSize: 10, Timeout: 5 * time.Second},
		user.SendWelcome)
	workers := make(chan struct{})
	go func() { wp.Run(ctx); close(workers) }()

	api := newAPI(cfg)
	api.Group("/v1", xttp.BearerTokens(cfg.APITokens...)).Add(user.Routes(welcome.Submit)...)

	slog.InfoContext(ctx, "starting", "version", version, "addr", cfg.Addr)
	err := api.Run(ctx, cfg.Addr) // returns after the signal and the drain
	cancel()
	<-workers
	return err
}

// newAPI is the hardened server every service starts from.
func newAPI(cfg ServeConfig) *xttp.API {
	return xttp.New(xttp.Config{
		Info:              xttp.Info{Title: "Example", Version: version},
		Fiber:             fiber.Config{TrustProxy: cfg.TrustProxy},
		HelmetPreset:      xttp.HelmetStrict,
		StrictContentType: true,
		RateLimit:         cfg.RateLimit,
		DefaultErrors:     []int{400, 401, 404, 415, 429, 500},
		SecuritySchemes:   map[string]any{"bearer": map[string]any{"type": "http", "scheme": "bearer"}},
		Docs:              xttp.DocsConfig{Enabled: cfg.Docs},
		Listen:            fiber.ListenConfig{DisableStartupMessage: true},
	})
}
