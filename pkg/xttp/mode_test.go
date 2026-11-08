package xttp

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestModeRawMountsNothing(t *testing.T) {
	api := New(Config{Mode: ModeRaw})
	api.Add(Route{Method: fiber.MethodGet, Path: "/x", Handler: func(c fiber.Ctx) error { return c.SendString("ok") }})
	res, err := api.App().Test(httptest.NewRequest(fiber.MethodGet, "/x", nil))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || res.Header.Get("X-Content-Type-Options") != "" || res.Header.Get("X-Request-Id") != "" {
		t.Errorf("status %d, headers %v", res.StatusCode, res.Header)
	}
	res, err = api.App().Test(httptest.NewRequest(fiber.MethodGet, "/livez", nil))
	if err != nil || res.StatusCode != 404 {
		t.Errorf("/livez = %v, %v; want 404", res, err)
	}
}

func TestModeRawRejectsConflicts(t *testing.T) {
	yes := true
	for name, cfg := range map[string]Config{
		"RateLimit":  {RateLimit: 10},
		"AccessLog":  {AccessLog: &yes},
		"Health":     {Health: Health{Live: func(fiber.Ctx) bool { return true }}},
		"Profiling":  {Profiling: ProfilingConfig{Enabled: true, AllowUnguarded: true}},
		"OnResponse": {OnResponse: func(fiber.Ctx, int, error) {}},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("New did not panic")
				}
			}()
			cfg.Mode = ModeRaw
			New(cfg)
		})
	}
}
