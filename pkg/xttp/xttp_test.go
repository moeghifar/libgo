package xttp

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

type Address struct {
	City string `json:"city"`
}

type Base struct {
	ID int `json:"id"`
}

type User struct {
	Base
	Name     string         `json:"name" doc:"Full name"`
	Email    string         `json:"email,omitempty"`
	Born     time.Time      `json:"born"`
	Address  *Address       `json:"address,omitempty"`
	Tags     []string       `json:"tags"`
	Meta     map[string]int `json:"meta,omitzero"`
	Manager  *User          `json:"manager,omitempty"`
	Secret   string         `json:"-"`
	internal string         //nolint:unused // proves unexported fields are skipped
}

type ListQuery struct {
	Page  int    `query:"page" doc:"Page number"`
	Sort  string `query:"sort" required:"true"`
	Other string
}

type APIError struct {
	Message string `json:"message"`
}

func newAPI() (*fiber.App, *API) {
	api := New(Config{Info: Info{Title: "Test", Version: "1.0.0"}})
	app := api.App()
	return app, api
}

func get(t *testing.T, m any, path ...string) any {
	t.Helper()
	for _, p := range path {
		mm, ok := m.(map[string]any)
		if !ok {
			t.Fatalf("at %q: not a map: %#v", p, m)
		}
		m = mm[p]
	}
	return m
}

func TestHandlerIsUntouched(t *testing.T) {
	app, api := newAPI()
	api.Group("/v1").Add(Route{
		Method: fiber.MethodGet, Path: "/users/:id",
		Handler: func(c fiber.Ctx) error {
			c.Set("X-Custom", "yes")
			return c.Status(fiber.StatusTeapot).SendString("user " + c.Params("id"))
		},
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/users/42", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusTeapot || string(body) != "user 42" || resp.Header.Get("X-Custom") != "yes" {
		t.Fatalf("got %d %q %q", resp.StatusCode, body, resp.Header.Get("X-Custom"))
	}
}

func TestMiddlewaresRun(t *testing.T) {
	app, api := newAPI()
	var order []string
	mw := func(name string) fiber.Handler {
		return func(c fiber.Ctx) error { order = append(order, name); return c.Next() }
	}
	api.Group("/g", mw("group")).Add(Route{
		Method: fiber.MethodGet, Path: "/x", Middlewares: []fiber.Handler{mw("route")},
		Handler: func(c fiber.Ctx) error { order = append(order, "handler"); return nil },
	})
	if _, err := app.Test(httptest.NewRequest("GET", "/g/x", nil)); err != nil {
		t.Fatal(err)
	}
	if want := []string{"group", "route", "handler"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestOperation(t *testing.T) {
	_, api := newAPI()
	api.Group("/v1").Add(Route{
		Method: fiber.MethodPost, Path: "/orgs/:org/users/:id?",
		Summary: "Create", OperationID: "createUser", Tags: []string{"users"}, Deprecated: true,
		Query: ListQuery{}, Body: User{},
		Responses: map[int]any{201: User{}, 204: nil, 400: APIError{}},
		Handler:   func(c fiber.Ctx) error { return nil },
	})
	spec := api.Spec()
	op := get(t, spec, "paths", "/v1/orgs/{org}/users/{id}", "post")
	if op == nil {
		t.Fatalf("operation missing; paths = %v", get(t, spec, "paths"))
	}

	tests := []struct {
		name string
		path []string
		want any
	}{
		{"summary", []string{"summary"}, "Create"},
		{"operationId", []string{"operationId"}, "createUser"},
		{"deprecated", []string{"deprecated"}, true},
		{"body ref", []string{"requestBody", "content", "application/json", "schema", "$ref"}, "#/components/schemas/User"},
		{"201 ref", []string{"responses", "201", "content", "application/json", "schema", "$ref"}, "#/components/schemas/User"},
		{"204 no body", []string{"responses", "204", "content"}, nil},
		{"204 description", []string{"responses", "204", "description"}, "No Content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := get(t, op, tt.path...); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}

	params := get(t, op, "parameters").([]any)
	var names []string
	for _, p := range params {
		pm := p.(map[string]any)
		names = append(names, pm["in"].(string)+":"+pm["name"].(string))
	}
	if want := []string{"path:org", "path:id", "query:page", "query:sort"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("params = %v, want %v", names, want)
	}
	if params[3].(map[string]any)["required"] != true || params[2].(map[string]any)["description"] != "Page number" {
		t.Fatalf("query tags not applied: %v", params[2:])
	}
}

func TestSchema(t *testing.T) {
	_, api := newAPI()
	api.Add(Route{Method: "GET", Path: "/u", Responses: map[int]any{200: []User{}}, Handler: func(fiber.Ctx) error { return nil }})
	spec := api.Spec()
	user := get(t, spec, "components", "schemas", "User")

	tests := []struct {
		name string
		path []string
		want any
	}{
		{"embedded flattened", []string{"properties", "id", "type"}, "integer"},
		{"doc tag", []string{"properties", "name", "description"}, "Full name"},
		{"time", []string{"properties", "born", "format"}, "date-time"},
		{"pointer struct ref", []string{"properties", "address", "$ref"}, "#/components/schemas/Address"},
		{"slice", []string{"properties", "tags", "items", "type"}, "string"},
		{"map", []string{"properties", "meta", "additionalProperties", "type"}, "integer"},
		{"recursive ref", []string{"properties", "manager", "$ref"}, "#/components/schemas/User"},
		{"json dash skipped", []string{"properties", "Secret"}, nil},
		{"unexported skipped", []string{"properties", "internal"}, nil},
		{"required", []string{"required"}, []any{"id", "name", "born", "tags"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := get(t, user, tt.path...); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
	if got := get(t, spec, "paths", "/u", "get", "responses", "200", "content", "application/json", "schema", "items", "$ref"); got != "#/components/schemas/User" {
		t.Fatalf("array response = %v", got)
	}
}

func TestAddPanicsOnIncompleteRoute(t *testing.T) {
	_, api := newAPI()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	api.Add(Route{Method: "GET", Path: "/x"})
}

func TestDocsServed(t *testing.T) {
	app, api := newAPI()
	api.Add(Route{Method: "GET", Path: "/users/:id", Responses: map[int]any{200: User{}}, Handler: func(fiber.Ctx) error { return nil }})
	api.Docs("/docs")

	tests := []struct {
		path, contentType, contains string
	}{
		{"/docs", "text/html", `data-url="/docs/openapi.json"`},
		{"/docs/openapi.yaml", "application/yaml", `"/users/{id}":`},
		{"/docs/openapi.json", "application/json", `"openapi": "3.1.0"`},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("GET", tt.path, nil))
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), tt.contentType) || !strings.Contains(string(body), tt.contains) {
				t.Fatalf("got %d %q\n%s", resp.StatusCode, resp.Header.Get("Content-Type"), body)
			}
		})
	}
}

func TestYAML(t *testing.T) {
	got := string(marshalYAML(map[string]any{
		"b":       []any{"x", map[string]any{"k": 1, "j": true}},
		"a":       map[string]any{},
		"/p/{id}": "s: \"q\"",
		"200":     []any{},
	}))
	want := `"/p/{id}": "s: \"q\""
"200": []
a: {}
b:
  - "x"
  -
    j: true
    k: 1
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestJSONMatchesSpec(t *testing.T) {
	_, api := newAPI()
	api.Add(Route{Method: "GET", Path: "/u", Handler: func(fiber.Ctx) error { return nil }})
	var m map[string]any
	if err := json.Unmarshal(api.JSON(), &m); err != nil || m["openapi"] != "3.1.0" {
		t.Fatalf("json = %v, err = %v", m, err)
	}
}

func TestDefaultMiddlewares(t *testing.T) {
	api := New(Config{Info: Info{Title: "t", Version: "1"}})
	app := api.App()
	api.Add(Route{Method: fiber.MethodGet, Path: "/boom", Handler: func(fiber.Ctx) error { panic("boom") }})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/boom", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("panic: status %d, want 500", resp.StatusCode)
	}
	if resp.Header.Get(fiber.HeaderXRequestID) == "" {
		t.Error("no request id header")
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("no helmet headers")
	}
}

func TestErrorResponsesDocumented(t *testing.T) {
	api := New(Config{DefaultErrors: []int{400}})
	api.Add(
		Route{Method: "GET", Path: "/a", Responses: map[int]any{200: User{}, 404: nil}, Handler: func(fiber.Ctx) error { return nil }},
		Route{Method: "GET", Path: "/b", Responses: map[int]any{400: APIError{}}, Handler: func(fiber.Ctx) error { return nil }},
	)
	spec := api.Spec()
	ref := "#/components/schemas/ErrorDocument"
	if got := get(t, spec, "paths", "/a", "get", "responses", "404", "content", MIMEJSONAPI, "schema", "$ref"); got != ref {
		t.Errorf("404 = %v", got)
	}
	if got := get(t, spec, "paths", "/a", "get", "responses", "400", "content", MIMEJSONAPI, "schema", "$ref"); got != ref {
		t.Errorf("default 400 = %v", got)
	}
	if got := get(t, spec, "paths", "/b", "get", "responses", "400", "content", "application/json", "schema", "$ref"); got != "#/components/schemas/APIError" {
		t.Errorf("route-set 400 = %v", got)
	}
	if got := get(t, spec, "components", "schemas", "ErrorDocument", "required"); !reflect.DeepEqual(got, []any{"errors"}) {
		t.Errorf("ErrorDocument required = %v", got)
	}
}

func TestRequiredTag(t *testing.T) {
	type Body struct {
		A string
		B string `required:"false"`
		C string `json:",omitempty" required:"true"`
	}
	_, api := newAPI()
	api.Add(Route{Method: "POST", Path: "/x", Body: Body{}, Handler: func(fiber.Ctx) error { return nil }})
	if got := get(t, api.Spec(), "components", "schemas", "Body", "required"); !reflect.DeepEqual(got, []any{"A", "C"}) {
		t.Fatalf("required = %v", got)
	}
}

func TestGroupRoot(t *testing.T) {
	app, api := newAPI()
	api.Group("/v1/keys").Add(Route{Method: "GET", Path: "", Handler: func(c fiber.Ctx) error { return c.SendStatus(200) }})
	if resp, err := app.Test(httptest.NewRequest("GET", "/v1/keys", nil)); err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", resp, err)
	}
	if get(t, api.Spec(), "paths", "/v1/keys", "get") == nil {
		t.Fatal("spec has no /v1/keys")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on root API")
		}
	}()
	api.Add(Route{Method: "GET", Path: "", Handler: func(fiber.Ctx) error { return nil }})
}
