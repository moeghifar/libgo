package xttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

var errUserNotFound = Error{Status: 404, Code: "USR-404", Title: "User not found"}

func TestErrorHandler(t *testing.T) {
	cause := errors.New("sql: no rows; dsn=secret")
	tests := []struct {
		name   string
		path   string
		err    error
		status int
		want   []map[string]any // subset of each error object
		hidden string           // must not appear in the body
	}{
		{"declared", "/x", errUserNotFound.With("no user 42", cause), 404,
			[]map[string]any{{"status": "404", "code": "USR-404", "title": "User not found", "detail": "no user 42"}}, "dsn=secret"},
		{"wrapped", "/x", fmt.Errorf("svc: %w", errUserNotFound.With("", nil)), 404,
			[]map[string]any{{"status": "404", "code": "USR-404"}}, ""},
		{"unknown", "/x", cause, 500,
			[]map[string]any{{"status": "500", "title": "Internal Server Error"}}, "dsn=secret"},
		{"fiber 4xx", "/x", fiber.ErrMethodNotAllowed, 405,
			[]map[string]any{{"status": "405", "title": "Method Not Allowed"}}, ""},
		{"no status", "/x", (&Error{Title: "oops"}), 500,
			[]map[string]any{{"status": "500", "title": "oops"}}, ""},
		{"many 4xx", "/x", Errors{
			{Status: 422, Code: "VAL", Source: &ErrorSource{Pointer: "/data/attributes/email"}},
			{Status: 409, Code: "DUP"},
		}, 400, []map[string]any{{"status": "422", "code": "VAL"}, {"status": "409", "code": "DUP"}}, ""},
		{"unknown route", "/missing", nil, 404,
			[]map[string]any{{"status": "404", "title": "Not Found"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := New(Config{Info: Info{Title: "t", Version: "1"}})
			app := api.App()
			api.Add(Route{Method: fiber.MethodGet, Path: "/x", Handler: func(fiber.Ctx) error { return tt.err }})

			resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, tt.path, nil))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tt.status {
				t.Errorf("status %d, want %d", resp.StatusCode, tt.status)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, MIMEJSONAPI) {
				t.Errorf("content-type %q", ct)
			}
			var raw strings.Builder
			var doc struct{ Errors []map[string]any }
			if err := json.NewDecoder(io.TeeReader(resp.Body, &raw)).Decode(&doc); err != nil {
				t.Fatal(err)
			}
			if tt.hidden != "" && strings.Contains(raw.String(), tt.hidden) {
				t.Errorf("body leaks %q: %s", tt.hidden, raw.String())
			}
			if len(doc.Errors) != len(tt.want) {
				t.Fatalf("got %d errors, want %d: %s", len(doc.Errors), len(tt.want), raw.String())
			}
			for i, w := range tt.want {
				for k, v := range w {
					if doc.Errors[i][k] != v {
						t.Errorf("errors[%d].%s = %v, want %v", i, k, doc.Errors[i][k], v)
					}
				}
				if doc.Errors[i]["id"] == "" || doc.Errors[i]["id"] == nil {
					t.Errorf("errors[%d] has no id (request id)", i)
				}
			}
		})
	}
}

func TestErrorWithCopies(t *testing.T) {
	a := errUserNotFound.With("a", nil)
	if errUserNotFound.Detail != "" || a.Detail != "a" {
		t.Fatal("With mutated the shared value")
	}
}
