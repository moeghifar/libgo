// Command strict runs an xttp server with the production security profile:
// HelmetStrict, no docs, no profiling, no rate-limit headers, JSON bodies only.
// It is the target for .specify/research/008-vapt-strict.sh:
//
//	go run ./example/strict -addr 127.0.0.1:18443
//	X=http://127.0.0.1:18443 S=http://127.0.0.1:18443 bash .specify/research/008-vapt-strict.sh
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/moeghifar/libgo/pkg/xttp"
)

// Key is a created key.
type Key struct {
	ID    string `json:"id"`
	Label string `json:"label" required:"false"`
}

var errInvalidKey = xttp.Error{Status: fiber.StatusBadRequest, Code: "KEY-400", Title: "Invalid key"}

func main() {
	addr := flag.String("addr", "127.0.0.1:18443", "listen address")
	flag.Parse()

	api := xttp.New(xttp.Config{
		Info:              xttp.Info{Title: "Strict", Version: "0.2.0"},
		HelmetPreset:      xttp.HelmetStrict,
		StrictContentType: true,
		DefaultErrors:     []int{400, 404, 500},
		RateLimit:         1000,
		Listen:            fiber.ListenConfig{DisableStartupMessage: true},
	})
	v1 := api.Group("/v1")
	v1.Add(
		xttp.Route{Method: fiber.MethodGet, Path: "/health", Summary: "Health", Responses: map[int]any{200: nil},
			Handler: func(c fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) }},
		xttp.Route{Method: fiber.MethodPost, Path: "/keys", Summary: "Create a key", Body: Key{}, Responses: map[int]any{201: Key{}},
			Handler: func(c fiber.Ctx) error {
				var k Key
				if err := c.Bind().JSON(&k); err != nil {
					return errInvalidKey.With("body must be a JSON key", err)
				}
				k.ID = "k1"
				return c.Status(fiber.StatusCreated).JSON(k)
			}},
	)
	if err := api.Run(context.Background(), *addr); err != nil {
		slog.Error("serve", "error", err)
		os.Exit(1)
	}
}
