package main

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFiberInterop(t *testing.T) {
	app := newAPI().App()
	cases := []struct {
		name, path, token string
		status            int
		body              string // substring
	}{
		{"3 group auth rejects", "/v1/me", "", 401, `"code":"AUTH-401"`},
		{"3 group auth passes", "/v1/me", "user-token", 200, `"subject":"u2"`},
		{"4 role denied", "/v1/admin/stats", "user-token", 403, `"code":"AUTH-403"`},
		{"4 role allowed", "/v1/admin/stats", "admin-token", 200, `"users":2`},
		{"5 timeout-wrapped handler", "/v1/report", "user-token", 200, ""},
		{"6 plain fiber route", "/plain", "", 200, "plain fiber"},
		{"7 net/http adaptor", "/v1/legacy", "user-token", 200, "from net/http"},
		{"8 mounted fiber app", "/old/ping", "", 200, "pong"},
		{"unknown route stays JSON:API", "/nope", "", 404, `"code":"HTTP-404"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(res.Body)
			if res.StatusCode != tc.status || !strings.Contains(string(b), tc.body) {
				t.Fatalf("got %d %s", res.StatusCode, b)
			}
			// 1. global custom middleware and xttp's headers reach every route.
			if res.Header.Get("X-Service") == "" && tc.status == 200 {
				t.Error("missing X-Service from global middleware")
			}
			if res.Header.Get("Content-Security-Policy") == "" {
				t.Error("missing xttp security headers")
			}
		})
	}

	// 4. per-route limiter: admin gets 5/min; 4 more pass, the next is 429.
	for i := range 5 {
		req := httptest.NewRequest("GET", "/v1/admin/stats", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		res, _ := app.Test(req)
		if want := map[bool]int{true: 429, false: 200}[i == 4]; res.StatusCode != want {
			t.Fatalf("request %d: got %d want %d", i, res.StatusCode, want)
		}
	}
}
