//go:build xttp_pprof

package xttp

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/pprof"
)

const profilingBuilt = true

func mountProfiling(app *fiber.App, p *ProfilingConfig) {
	chain := append([]any{p.Prefix + "/debug/pprof"}, handlers(p.Guard)...)
	app.Use(append(chain, pprof.New(pprof.Config{Prefix: p.Prefix}))...)
}
