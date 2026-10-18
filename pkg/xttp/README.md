# xttp

Fiber v3 routes that also describe themselves as an OpenAPI 3.1 document.

It looks like Huma from the outside, one struct per operation with OpenAPI generated from Go types, but it does not wrap your handler. Handlers stay plain `fiber.Handler`: you bind the request and write the response yourself, with all of Fiber's control. The types on a route only describe it.

## Features

- **Production server by default**: `xttp.New(Config)` builds the Fiber app; see [Server defaults](#server-defaults)
- **Plain handlers**: `func(c fiber.Ctx) error`, no input/output wrapper structs, no hidden binding or encoding
- **One struct per route**: `Route{Method, Path, Summary, Tags, Query, Body, Responses, Handler}`
- **Schemas from Go types**: structs, pointers, slices, maps, `time.Time`, `[]byte`, embedded structs, recursive types
- **`encoding/json` rules**: `json` names, `-` skipped, `omitempty`/`omitzero` mean optional, everything else is required
- **Components and `$ref`**: named structs are written once under `components/schemas`
- **Parameters**: Fiber `:id` / `:id?` become `{id}` path params; a `Query` struct's `query:"name"` fields become query params
- **Groups**: `api.Group("/v1", mw...)` prefixes both the Fiber routes and the document paths
- **Per-route middlewares**: `Route.Middlewares` run before the handler
- **YAML and JSON**: `api.YAML()` / `api.JSON()`, plus `api.Docs("/docs")` serving a Scalar page, `/docs/openapi.yaml` and `/docs/openapi.json`
- **Startup-only reflection**: schemas are built when a route is added, so a request pays nothing
- **No YAML dependency**: a small stdlib emitter writes the document

## Usage

```go
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name" doc:"Full name"`
	Email string `json:"email,omitempty"`
}

type ListUsers struct {
	Page int    `query:"page" doc:"Page number"`
	Sort string `query:"sort" required:"true"`
}

var ErrBadInput = xttp.Error{Status: 400, Code: "USR-400", Title: "Invalid input"}

api := xttp.New(xttp.Config{Info: xttp.Info{Title: "Users", Version: "1.0.0"}})
v1 := api.Group("/v1")

v1.Add(
	xttp.Route{
		Method:    fiber.MethodGet,
		Path:      "/users",
		Summary:   "List users",
		Tags:      []string{"users"},
		Query:     ListUsers{},
		Responses: map[int]any{200: []User{}},
		Handler: func(c fiber.Ctx) error {
			var q ListUsers
			if err := c.Bind().Query(&q); err != nil {
				return ErrBadInput.With(err.Error(), err)
			}
			return c.JSON([]User{})
		},
	},
	xttp.Route{
		Method:    fiber.MethodPost,
		Path:      "/users",
		Summary:   "Create a user",
		Tags:      []string{"users"},
		Body:      User{},
		Responses: map[int]any{201: User{}},
		Handler: func(c fiber.Ctx) error {
			var u User
			if err := c.Bind().JSON(&u); err != nil {
				return ErrBadInput.With(err.Error(), err)
			}
			return c.Status(fiber.StatusCreated).JSON(u)
		},
	},
)

api.Docs("/docs") // add routes first: the document renders on the first request
api.Run(context.Background(), ":3000") // graceful stop on SIGINT/SIGTERM
```

Export the spec without a server, for CI or a committed file:

```go
os.WriteFile("openapi.yaml", api.YAML(), 0o644)
```

## Server defaults

`xttp.New(xttp.Config{...})` creates the Fiber app. Every field is optional; zero means the default.

| Setting | Default | Change it |
| --- | --- | --- |
| Read / write / idle timeout | 10s / 30s / 120s | `Fiber.ReadTimeout` etc.; negative read/write = off |
| Body limit | 1 MiB (413 above) | `Fiber.BodyLimit` |
| Rate limit | 100 req/s per client IP, sliding window, 429 JSON:API | `RateLimit: n`; `-1` = off. Behind a proxy set `Fiber.TrustProxy` |
| CORS | off | `CORS: &cors.Config{AllowOrigins: ...}` |
| Probes | `GET /livez`, `/readyz`, `/startupz` → 200 / 503 | `Health{Live, Ready, Startup func(fiber.Ctx) bool}` |
| Request ID | `X-Request-ID`; incoming kept only if ≤ 72 chars (two UUIDs) of `[A-Za-z0-9._-]`, else generated | — |
| Errors / 404 | always `xttp.ErrorHandler` (JSON:API); unknown route → `HTTP-404` | — |
| Shutdown | `Run` stops on ctx done, SIGINT or SIGTERM; `/readyz` → 503; waits 10s for in-flight requests | `Listen.ShutdownTimeout`; TLS via `Listen.CertFile`… |

Middleware order: probes (not logged, not limited) → request id → slog access log → recover → helmet → CORS → rate limit → your routes.

Cleanup on shutdown uses Fiber's hooks:

```go
api.App().Hooks().OnPostShutdown(func(err error) error { return db.Close() })
```

Fiber v3 can call `OnPostShutdown` twice (seen on v3.3.0) on a graceful stop, so make cleanup idempotent (`sync.Once`).

## Errors (JSON:API)

Every error leaves as a [JSON:API error document](https://jsonapi.org/format/#error-objects), `Content-Type: application/vnd.api+json`:

```go

var ErrUserNotFound = xttp.Error{Status: 404, Code: "USR-404", Title: "User not found"}

func getUser(c fiber.Ctx) error {
	u, err := repo.Find(c.Params("id"))
	if err != nil {
		return ErrUserNotFound.With("no user with id "+c.Params("id"), err) // err is logged, never sent
	}
	return c.JSON(u)
}
```

```json
{"errors":[{"id":"<request id>","status":"404","code":"USR-404","title":"User not found","detail":"no user with id 42"}]}
```

- `With` returns a copy; only `*Error` is an `error`, so a shared value can't be returned or mutated by accident
- `Unwrap` works: `fmt.Errorf("svc: %w", e)` still renders `e`, `errors.As` finds it
- `xttp.Errors{...}` sends several at once (validation); status is the shared one, else 400 for all-4xx, else 500
- `Source{Pointer|Parameter|Header}` and `Meta` as in the spec
- Fiber errors (unknown route 404, 405, 413) keep their status; any other error is a 500 with no internal text in the body, logged with slog
- Safe without middlewares: requests fasthttp rejects before routing still get a valid document

## Struct tags

| Tag        | Where        | Meaning                                       |
| ---------- | ------------ | --------------------------------------------- |
| `json`     | body/response | property name, `-`, `omitempty`, `omitzero` |
| `doc`      | any field    | description                                   |
| `query`    | `Query` struct | query parameter name (same tag Fiber binds) |
| `required` | `Query` struct | `"true"` marks the parameter required       |

## What it does not do

No validation, no request binding, no security schemes. Use Fiber's `c.Bind()`. Add these when a real route needs them.

## License

Apache 2.0, see [LICENSE](../../LICENSE)
