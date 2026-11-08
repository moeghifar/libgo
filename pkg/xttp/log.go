package xttp

import (
	"log/slog"
	"mime"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// observe runs after the handler, so the status is the real one: it writes the
// access log and calls Config.OnResponse. The log never holds bodies, headers
// or query strings.
func (a *API) observe(c fiber.Ctx) error {
	start := time.Now()
	err := a.mapped(c, c.Next())
	status := c.Response().StatusCode()
	if err != nil {
		status = StatusOf(err) // the error handler sets the status only after middlewares return
	}
	if a.accessLog {
		level := slog.LevelInfo
		if status >= fiber.StatusInternalServerError {
			level = slog.LevelError
		}
		if ctx := c.Context(); a.log.Enabled(ctx, level) {
			// LogAttrs: typed attrs, no interface boxing per value.
			a.log.LogAttrs(ctx, level, "http request",
				slog.String("request_id", requestid.FromContext(c)),
				slog.String("method", c.Method()),
				slog.String("path", c.Path()),
				slog.Int("status", status),
				slog.Duration("latency", time.Since(start)),
				slog.String("ip", c.IP()),
			)
		}
	}
	if a.onResponse != nil {
		func() {
			defer a.contain(c, "OnResponse")
			a.onResponse(c, status, err)
		}()
	}
	return err
}

// strictContentType rejects a request body whose media type is not
// application/json with 415. Bodiless requests pass, so method routing still
// answers 405.
func strictContentType(c fiber.Ctx) error {
	if len(c.Body()) > 0 {
		mt, _, err := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
		if err != nil || mt != fiber.MIMEApplicationJSON {
			return fiber.ErrUnsupportedMediaType
		}
	}
	return c.Next()
}
