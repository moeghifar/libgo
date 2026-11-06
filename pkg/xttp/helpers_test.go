package xttp

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func call(t *testing.T, api *API, method, path, body string, headers map[string]string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := api.App().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

func TestBind(t *testing.T) {
	type in struct {
		N int `json:"n"`
	}
	tests := []struct {
		name, body string
		status     int
		contains   string
	}{
		{"valid", `{"n":1}`, 200, ""},
		{"empty", ``, 200, ""},
		{"unknown field", `{"n":1,"x":2}`, 400, ""},
		{"trailing data", `{"n":1} {"n":2}`, 400, "after the JSON"},
		{"wrong type", `{"n":"a"}`, 400, `"pointer":"/n"`},
		{"broken", `{`, 400, ""},
	}
	api := New(Config{AccessLog: new(bool)})
	api.Add(Route{Method: "POST", Path: "/x", Handler: func(c fiber.Ctx) error {
		var v in
		if err := Bind(c, &v); err != nil {
			return err
		}
		return c.SendStatus(200)
	}})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := call(t, api, "POST", "/x", tt.body, nil)
			if status != tt.status || !strings.Contains(body, tt.contains) {
				t.Fatalf("got %d %s", status, body)
			}
		})
	}
}

func TestRequiredQueryRunsAfterAuth(t *testing.T) {
	type q struct {
		ID   string `query:"id" required:"true"`
		Page int    `query:"page"`
	}
	api := New(Config{})
	api.Add(Route{Method: "GET", Path: "/x", Query: q{}, Middlewares: []fiber.Handler{BearerTokens("t")},
		Handler: func(c fiber.Ctx) error {
			var v q
			if err := BindQuery(c, &v); err != nil {
				return err
			}
			return c.SendString(v.ID)
		}})
	auth := map[string]string{"Authorization": "Bearer t"}
	if s, _ := call(t, api, "GET", "/x", "", nil); s != 401 {
		t.Fatalf("no auth: %d", s)
	}
	if s, b := call(t, api, "GET", "/x", "", auth); s != 400 || !strings.Contains(b, `"parameter":"id"`) {
		t.Fatalf("missing: %d %s", s, b)
	}
	if s, _ := call(t, api, "GET", "/x?id=1&page=nope", "", auth); s != 400 {
		t.Fatalf("bad page: %d", s)
	}
	if s, b := call(t, api, "GET", "/x?id=1", "", auth); s != 200 || b != "1" {
		t.Fatalf("ok: %d %s", s, b)
	}
}

func TestMapErrorAndOnResponse(t *testing.T) {
	errGone := errors.New("gone")
	notFound := Error{Status: 404, Code: "X-404", Title: "Gone"}
	var status []int
	api := New(Config{
		AccessLog: new(bool),
		MapError: func(err error) error {
			switch {
			case errors.Is(err, errGone):
				return notFound.With("", err)
			case err.Error() == "hook":
				panic("bad hook")
			}
			return nil
		},
		OnResponse: func(_ fiber.Ctx, s int, _ error) { status = append(status, s); panic("audit down") },
	})
	api.Add(
		Route{Method: "GET", Path: "/mapped", Handler: func(fiber.Ctx) error { return errGone }},
		Route{Method: "GET", Path: "/plain", Handler: func(fiber.Ctx) error { return errors.New("secret text") }},
		Route{Method: "GET", Path: "/hook", Handler: func(fiber.Ctx) error { return errors.New("hook") }},
		Route{Method: "GET", Path: "/panic", Handler: func(fiber.Ctx) error { panic("boom") }},
		Route{Method: "GET", Path: "/ok", Handler: func(c fiber.Ctx) error { return c.SendStatus(204) }},
	)
	for path, want := range map[string]int{"/mapped": 404, "/plain": 500, "/hook": 500, "/panic": 500, "/ok": 204} {
		status = nil
		got, body := call(t, api, "GET", path, "", nil)
		if got != want || len(status) != 1 || status[0] != want || strings.Contains(body, "secret") {
			t.Errorf("%s: got %d, hook %v, body %s", path, got, status, body)
		}
	}
	if err := notFound.With("", errGone); !IsCode(err, "X-404") || CodeOf(errGone) != "" {
		t.Error("CodeOf")
	}
}

func TestCredentials(t *testing.T) {
	tests := []struct {
		name    string
		mw      fiber.Handler
		headers map[string]string
		status  int
	}{
		{"second token", BearerTokens("a", "b"), map[string]string{"Authorization": "bearer b"}, 200},
		{"wrong token", BearerTokens("a"), map[string]string{"Authorization": "Bearer ab"}, 401},
		{"wrong scheme", BearerTokens("a"), map[string]string{"Authorization": "Basic a"}, 401},
		{"no header", BearerTokens("a"), nil, 401},
		{"key", APIKey("X-Key", "k"), map[string]string{"X-Key": "k"}, 200},
		{"no key", APIKey("X-Key", "k"), nil, 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := New(Config{AccessLog: new(bool)})
			api.Add(Route{Method: "GET", Path: "/x", Middlewares: []fiber.Handler{tt.mw},
				Handler: func(c fiber.Ctx) error { return c.SendStatus(200) }})
			if s, _ := call(t, api, "GET", "/x", "", tt.headers); s != tt.status {
				t.Fatalf("got %d", s)
			}
		})
	}
	for name, f := range map[string]func(){
		"no tokens": func() { BearerTokens() }, "empty token": func() { BearerTokens("") }, "no keys": func() { APIKey("X") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			f()
		}()
	}
}

func TestPrincipalAndSecurityDoc(t *testing.T) {
	api := New(Config{SecuritySchemes: map[string]any{"bearer": map[string]any{"type": "http", "scheme": "bearer"}}})
	api.Add(Route{Method: "GET", Path: "/me", Security: []string{"bearer"},
		Middlewares: []fiber.Handler{func(c fiber.Ctx) error {
			if _, ok := PrincipalOf(c); ok {
				t.Error("principal before SetPrincipal")
			}
			SetPrincipal(c, Principal{Subject: "ada"})
			return c.Next()
		}},
		Handler: func(c fiber.Ctx) error {
			p, _ := PrincipalOf(c)
			return c.SendString(p.Subject)
		}})
	if _, b := call(t, api, "GET", "/me", "", nil); b != "ada" {
		t.Fatalf("body %q", b)
	}
	for _, doc := range []string{string(api.JSON()), string(api.YAML())} {
		if !strings.Contains(doc, "securitySchemes") || !strings.Contains(doc, "security") {
			t.Fatalf("no security in document:\n%s", doc)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("undeclared scheme did not panic")
		}
	}()
	api.Add(Route{Method: "GET", Path: "/y", Security: []string{"nope"}, Handler: func(fiber.Ctx) error { return nil }})
}
