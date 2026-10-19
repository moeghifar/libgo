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
- **YAML and JSON**: `api.YAML()` / `api.JSON()`, plus `Docs{Enabled: true}` (or `api.Docs("/docs")`) serving a Scalar page, `/docs/openapi.yaml` and `/docs/openapi.json`
- **Security headers always on**: helmet set, `Cache-Control: no-store`, no `Server` banner, also on 413/431 responses; `HelmetStrict` preset for pure APIs; see [Security defaults](#security-defaults)
- **JSON:API errors documented**: `xttp.ErrorDocument` is the wire type and the schema; `DefaultErrors` adds it to every route
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

api.Run(context.Background(), ":3000") // graceful stop on SIGINT/SIGTERM
```

Docs are opt-in: `Config{Docs: xttp.DocsConfig{Enabled: true}}` serves `/docs`, `/docs/openapi.yaml` and `/docs/openapi.json` (rendered on the first request, so route order does not matter); `api.Docs("/docs")` still works for manual mounting.

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
| Errors / 404 | always `(*API).ErrorHandler` (JSON:API); unknown route → `HTTP-404` | — |
| Helmet | always on: `helmet.ConfigDefault` + `Cache-Control: no-store`, no `Server` header | `HelmetPreset: xttp.HelmetStrict`; `Helmet: helmet.Config{...}` overrides one field, `"-"` removes a header; `Helmet.Next` skips a request |
| Headers | `Cache-Control: no-store` on every response, probes and 413/431 included | `Headers: map[string]string{"X-Robots-Tag": "noindex", "Cache-Control": "-"}` |
| HideServerHeader | true: no `Server` banner, also before routing | `HideServerHeader: &f` keeps `Fiber.ServerHeader` |
| Docs | off | `Docs: DocsConfig{Enabled: true, Path: "/docs", Page: &f, Guard: []fiber.Handler{...}}`; `Page: false` serves only the spec files (no CDN script) |
| Profiling | off | `Profiling: ProfilingConfig{Enabled: true, Guard: ...}`; a Guard is required (`New` panics without one) unless `AllowUnguarded` |
| Logger | `slog.Default()` | `Logger: *slog.Logger`, used by the access log and error handler |
| AccessLog | on, one line per request | `AccessLog: &f`; errors ≥ 500 are still logged |
| RateLimitHeaders | off: no `X-RateLimit-*` | `RateLimitHeaders: &t` |
| StrictContentType | off | `true`: a body whose media type is not `application/json` → 415 before any handler; bodiless requests pass |
| DefaultErrors | none | `DefaultErrors: []int{400, 404, 500}` documents `ErrorDocument` on every route that does not set the status |
| Listener | `tcp` (IPv4 + IPv6) | `Listen.ListenerNetwork: fiber.NetworkTCP4` |
| Shutdown | `Run` stops on ctx done, SIGINT or SIGTERM; `/readyz` → 503; waits 10s for in-flight requests | `Listen.ShutdownTimeout`; TLS via `Listen.CertFile`… |

Middleware order: probes (not logged, not limited) → request id → slog access log → recover → security headers → CORS → pprof → rate limit → content-type check → docs → your routes.

Middleware can read the status of a returned error with `xttp.StatusOf(err)`: the error handler sets the response status only after the chain returned.

## Security defaults

Always on, with no field to switch them off:

- Fiber helmet headers (`X-Content-Type-Options: nosniff`, `X-Frame-Options`, `Referrer-Policy: no-referrer`, COEP/COOP/CORP, ...) and `Cache-Control: no-store` on every response, including probes and the 413/431 responses fasthttp writes before routing
- no `Server` banner, no `X-Powered-By`
- JSON:API error bodies with no internal text; a 500's cause is logged, never sent
- request ids sanitised (≤ 72 chars of `[A-Za-z0-9._-]`), hostile values replaced
- body limit, read/write/idle timeouts and the per-IP rate limit (no `X-RateLimit-*` headers)
- docs and pprof off; pprof needs a guard

Header resolution, in order: preset → `Config.Helmet` field overrides → `"-"` removals → `Config.Headers`.

| Preset | CSP | X-Frame-Options | Permissions-Policy | CORP | HSTS (TLS only) |
| --- | --- | --- | --- | --- | --- |
| `HelmetDefault` | none | `SAMEORIGIN` | none | `same-origin` | none |
| `HelmetStrict` | `default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'` | `DENY` | every feature `()` | `same-origin` | `max-age=63072000; includeSubDomains; preload` |

The docs page sets its own `Content-Security-Policy` (exactly the Scalar script URL, inline styles, `connect-src 'self'`), so it renders under `HelmetStrict`. The CDN sends `Cross-Origin-Resource-Policy: cross-origin`, so helmet's `Cross-Origin-Embedder-Policy: require-corp` needs no relaxation (verified 2026-10-19 with `curl -sI`). Use `Page: false` in production.

[`example/strict`](../../example/strict) is the reference strict server; `.specify/research/008-vapt-strict.sh` runs the VAPT checklist against it.

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
- Safe without middlewares: requests fasthttp rejects before routing still get a valid document, with the security headers
- `xttp.ErrorDocument{Errors []ErrorObject}` is the wire type; a `Responses` entry ≥ 400 with `nil` or `ErrorDocument{}` is documented as `application/vnd.api+json` with that schema, and `Config.DefaultErrors` adds it to every route
- `xttp.StatusOf(err)` returns the status the handler will send (0 for nil), for audit or metrics middleware

## Struct tags

| Tag        | Where        | Meaning                                       |
| ---------- | ------------ | --------------------------------------------- |
| `json`     | body/response | property name, `-`, `omitempty`, `omitzero` |
| `doc`      | any field    | description                                   |
| `query`    | `Query` struct | query parameter name (same tag Fiber binds) |
| `required` | any field    | `"true"` / `"false"` overrides the default (query: optional; body: required unless `omitempty`/`omitzero`) |

## What it does not do

No validation, no request binding, no security schemes. Use Fiber's `c.Bind()`. Add these when a real route needs them.

## License

Apache 2.0, see [LICENSE](../../LICENSE)
