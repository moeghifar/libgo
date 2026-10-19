package xttp

import (
	"log/slog"
	"mime"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// accessLog writes one slog line per request after the handler ran, so the
// status is the real one. It never logs bodies, headers or query strings.
func (a *API) accessLog(c fiber.Ctx) error {
	start := time.Now()
	err := c.Next()
	status := c.Response().StatusCode()
	if err != nil {
		status = StatusOf(err) // the error handler sets the status only after middlewares return
	}
	level := slog.LevelInfo
	if status >= fiber.StatusInternalServerError {
		level = slog.LevelError
	}
	a.log.Log(c.Context(), level, "http request",
		"request_id", requestid.FromContext(c),
		"method", c.Method(),
		"path", c.Path(),
		"status", status,
		"latency", time.Since(start),
		"ip", c.IP(),
	)
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
