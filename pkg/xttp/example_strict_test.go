package xttp_test

import (
	"fmt"
	"net/http/httptest"

	"github.com/gofiber/fiber/v3"

	"github.com/moeghifar/libgo/pkg/xttp"
)

// The production profile: strict headers on every response, including the
// 404 below, no docs, no profiling, no rate-limit headers, JSON bodies only.
func ExampleNew_strict() {
	api := xttp.New(xttp.Config{
		Info:              xttp.Info{Title: "Strict", Version: "1.0.0"},
		HelmetPreset:      xttp.HelmetStrict,
		StrictContentType: true,
		Headers:           map[string]string{"X-Robots-Tag": "noindex"},
	})
	api.Add(xttp.Route{Method: fiber.MethodGet, Path: "/health", Handler: func(c fiber.Ctx) error { return c.SendStatus(200) }})

	resp, _ := api.App().Test(httptest.NewRequest("GET", "/nope", nil))
	for _, h := range []string{"Content-Security-Policy", "X-Frame-Options", "Permissions-Policy", "Cache-Control", "X-Robots-Tag", "Server", "X-RateLimit-Limit"} {
		fmt.Printf("%s: %q\n", h, resp.Header.Get(h))
	}
	// Output:
	// Content-Security-Policy: "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
	// X-Frame-Options: "DENY"
	// Permissions-Policy: "accelerometer=(), ambient-light-sensor=(), autoplay=(), battery=(), camera=(), display-capture=(), document-domain=(), encrypted-media=(), fullscreen=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), midi=(), payment=(), picture-in-picture=(), publickey-credentials-get=(), screen-wake-lock=(), sync-xhr=(), usb=(), web-share=(), xr-spatial-tracking=()"
	// Cache-Control: "no-store"
	// X-Robots-Tag: "noindex"
	// Server: ""
	// X-RateLimit-Limit: ""
}
