package xttp

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// accessLog writes one slog line per request after the handler ran, so the
// status is the real one. It never logs bodies, headers or query strings.
func accessLog(c fiber.Ctx) error {
	start := time.Now()
	err := c.Next()
	status := c.Response().StatusCode()
	if err != nil {
		// The error handler sets the status only after middlewares return.
		status = documentStatus(toErrors(err))
	}
	level := slog.LevelInfo
	if status >= fiber.StatusInternalServerError {
		level = slog.LevelError
	}
	slog.Log(c.Context(), level, "http request",
		"request_id", requestid.FromContext(c),
		"method", c.Method(),
		"path", c.Path(),
		"status", status,
		"latency", time.Since(start),
		"ip", c.IP(),
	)
	return err
}
