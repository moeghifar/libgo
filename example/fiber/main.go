// Command fiber shows that xttp is plain Fiber v3 underneath: every kind of
// Fiber middleware and handler you already have plugs in unchanged. Each
// variant is numbered and proven by main_test.go.
//
//	go run ./example/fiber
//	curl -H 'Authorization: Bearer admin-token' localhost:8080/v1/admin/stats
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/timeout"
	"github.com/moeghifar/libgo/pkg/xttp"
)

type Stats struct {
	Users int `json:"users"`
}

type Me struct {
	Subject string   `json:"subject"`
	Roles   []string `json:"roles"`
}

var (
	errUnauthorized = xttp.Error{Status: fiber.StatusUnauthorized, Code: "AUTH-401", Title: "Unauthorized"}
	errForbidden    = xttp.Error{Status: fiber.StatusForbidden, Code: "AUTH-403", Title: "Forbidden"}
)

// users stands in for your token store or JWT verifier.
var users = map[string]xttp.Principal{
	"admin-token": {Subject: "u1", Roles: []string{"admin"}},
	"user-token":  {Subject: "u2", Roles: []string{"user"}},
}

// authenticate is a hand-written Fiber middleware. Returning an xttp.Error
// renders the same JSON:API body as any handler error.
func authenticate(c fiber.Ctx) error {
	p, ok := users[strings.TrimPrefix(c.Get(fiber.HeaderAuthorization), "Bearer ")]
	if !ok {
		return errUnauthorized.With("missing or invalid bearer token", nil)
	}
	xttp.SetPrincipal(c, p)
	return c.Next()
}

// requireRole is a middleware factory, the usual Fiber pattern.
func requireRole(role string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if p, ok := xttp.PrincipalOf(c); !ok || !slices.Contains(p.Roles, role) {
			return errForbidden.With("needs role "+role, nil)
		}
		return c.Next()
	}
}

func newAPI() *xttp.API {
	api := xttp.New(xttp.Config{
		Info:              xttp.Info{Title: "Fiber interop", Version: "1.0.0"},
		HelmetPreset:      xttp.HelmetStrict,
		StrictContentType: true,
		SecuritySchemes:   map[string]any{"bearer": map[string]any{"type": "http", "scheme": "bearer"}},
		Listen:            fiber.ListenConfig{DisableStartupMessage: true},
	})
	app := api.App() // the real *fiber.App

	// 1. Global custom middleware, after xttp's own chain.
	app.Use(func(c fiber.Ctx) error {
		c.Set("X-Service", "fiber-interop")
		return c.Next()
	})
	// 2. Stock Fiber middleware, global.
	app.Use(compress.New())

	// 3. Group middleware: every /v1 route is authenticated.
	v1 := api.Group("/v1", authenticate)
	v1.Add(xttp.Route{
		Method: fiber.MethodGet, Path: "/me", Summary: "Current caller",
		Security: []string{"bearer"}, Responses: map[int]any{200: Me{}},
		Handler: func(c fiber.Ctx) error {
			p, _ := xttp.PrincipalOf(c)
			return c.JSON(Me{Subject: p.Subject, Roles: p.Roles})
		},
	})

	// 4. Route middleware: custom role check plus a stricter stock Fiber
	// limiter keyed by caller, on this route only.
	v1.Add(xttp.Route{
		Method: fiber.MethodGet, Path: "/admin/stats", Summary: "Admin stats",
		Security: []string{"bearer"}, Responses: map[int]any{200: Stats{}},
		Middlewares: []fiber.Handler{
			requireRole("admin"),
			limiter.New(limiter.Config{
				Max: 5, Expiration: time.Minute,
				KeyGenerator: func(c fiber.Ctx) string { p, _ := xttp.PrincipalOf(c); return p.Subject },
			}),
		},
		Handler: func(c fiber.Ctx) error { return c.JSON(Stats{Users: 2}) },
	})

	// 5. A Fiber handler wrapped by Fiber middleware (timeout) is still just
	// a fiber.Handler, so it goes straight into Route.Handler.
	v1.Add(xttp.Route{
		Method: fiber.MethodGet, Path: "/report", Summary: "Slow report",
		Security: []string{"bearer"}, Responses: map[int]any{200: nil},
		Handler: timeout.New(func(c fiber.Ctx) error {
			select {
			case <-time.After(10 * time.Millisecond):
				return c.SendStatus(fiber.StatusOK)
			case <-c.Context().Done():
				return c.Context().Err()
			}
		}, timeout.Config{Timeout: 2 * time.Second}),
	})

	// 6. A plain Fiber route, outside the OpenAPI document. Note: it still
	// gets xttp's global chain (headers, rate limit, recover, request id).
	app.Get("/plain", func(c fiber.Ctx) error { return c.SendString("plain fiber") })

	// 7. An existing net/http handler, through Fiber's adaptor.
	v1.Add(xttp.Route{
		Method: fiber.MethodGet, Path: "/legacy", Summary: "net/http handler",
		Security: []string{"bearer"}, Responses: map[int]any{200: nil},
		Handler: adaptor.HTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("from net/http"))
		}),
	})

	// 8. An existing *fiber.App, mounted as a sub-app.
	legacy := fiber.New()
	legacy.Get("/ping", func(c fiber.Ctx) error { return c.SendString("pong") })
	app.Use("/old", legacy)

	return api
}

func main() {
	if err := newAPI().Run(context.Background(), "127.0.0.1:8080"); err != nil {
		slog.Error("serve", "error", err)
		os.Exit(1)
	}
}
