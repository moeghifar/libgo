# xttp

Fiber v3 server with OpenAPI 3.1 generated from your types and hardened defaults.

Each route is one struct, and its Go types become the OpenAPI document, as in Huma. Unlike Huma, xttp does not wrap your handler: handlers stay plain `fiber.Handler`, so you bind the request and write the response yourself with all of Fiber's control. The types on a route only describe it. `xttp.New` also gives you a production server: security headers, rate limit, timeouts, probes, access log and JSON:API errors.

```sh
go get github.com/moeghifar/libgo/pkg/xttp
```

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
			if err := xttp.BindQuery(c, &q); err != nil {
				return err // a 400 JSON:API error
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
			if err := xttp.Bind(c, &u); err != nil {
				return err // unknown fields, trailing data or wrong types: 400
			}
			if u.Name == "" {
				return ErrBadInput.With("name is required", nil)
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
| Profiling | off | `Profiling: ProfilingConfig{Enabled: true, Guard: ...}`; a Guard is required, and the build needs `-tags xttp_pprof` (`New` panics without one) unless `AllowUnguarded` |
| Logger | `slog.Default()` | `Logger: *slog.Logger`, used by the access log and error handler |
| AccessLog | on, one line per request | `AccessLog: &f`; errors ≥ 500 are still logged |
| RateLimitHeaders | off: no `X-RateLimit-*` | `RateLimitHeaders: &t` |
| StrictContentType | off | `true`: a body whose media type is not `application/json` → 415 before any handler; bodiless requests pass |
| DefaultErrors | none | `DefaultErrors: []int{400, 404, 500}` documents `ErrorDocument` on every route that does not set the status |
| Listener | `tcp` (IPv4 + IPv6) | `Listen.ListenerNetwork: fiber.NetworkTCP4` |
| Shutdown | `Run` stops on ctx done, SIGINT or SIGTERM; `/readyz` → 503; waits 10s for in-flight requests | `Listen.ShutdownTimeout`; TLS via `Listen.CertFile`… |

Middleware order: probes (not logged, not limited) → request id → slog access log → recover → security headers → CORS → rate limit → content-type check → pprof → docs → your routes.

Middleware can read the status of a returned error with `xttp.StatusOf(err)`: the error handler sets the response status only after the chain returned.

## Mode

`Config.Mode` picks what `New` registers. `ModeDefault` (also an unset `Mode`) is everything on this page. `ModeRaw` registers no middleware and no probes, for a service that gets those from a proxy in front or adds its own:

```go
api := xttp.New(xttp.Config{Mode: xttp.ModeRaw})
```

In `ModeRaw` a handler panic crashes the process, and there are no security headers, rate limit, access log or request id. Timeouts, body limit, JSON:API errors and the OpenAPI document stay. `New` panics if you also set a field that needs the middleware.

To strip the unused middleware from the binary, build with the tag (Fiber itself remains):

```sh
go build -tags xttp_slim .
```

The tag only removes what your code does not name: `ModeRaw` is stripped and `ModeDefault` keeps everything. An unset `Mode` panics at startup in a slim build, so name the mode you want.

## Profiling

`Config.Profiling` is the one setting, and it works in any `Mode`. pprof is linked only into builds with the tag, so production binaries do not contain it:

```sh
go run -tags xttp_pprof .   # dev: Profiling.Enabled works
go build .                  # prod: pprof is not in the binary
```

Without the tag, `Profiling.Enabled: true` panics at startup rather than being ignored.

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
- `xttp.Error`, `Errors` and `ErrorSource` are aliases of [`xerr`](../xerr/README.md); code that serves no HTTP (workers, CLIs, gRPC) imports `xerr` and returns the same values without linking Fiber

## Binding, errors and auth helpers

`xttp.Bind(c, &in)` decodes JSON strictly: unknown fields, trailing data and wrong types are a 400. `xttp.BindQuery(c, &q)` binds the query. A `required:"true"` query parameter on `Route.Query` is enforced with a 400 naming it, after `Route.Middlewares`.

`Config.MapError` turns your own errors into `*xttp.Error` in one place; `CodeOf` and `IsCode` read the code back. `Config.OnResponse` gets the final status of every routed request, including panics, for audit logs and metrics.

`xttp.BearerTokens(tokens...)` and `xttp.APIKey(header, keys...)` compare in constant time and panic at startup on an empty list. Pair them with `Route.Security` and `Config.SecuritySchemes` to document them. Custom auth middleware records the caller with `SetPrincipal`; handlers read it with `PrincipalOf`.

## Outbound HTTP log

`xttp.OutboundLog` is an `http.RoundTripper` that logs every call your service makes to other APIs:

```go
client := &http.Client{Transport: &xttp.OutboundLog{}} // or OutboundLog{Next: yourTransport, Logger: l}

req, _ := http.NewRequestWithContext(c.Context(), http.MethodGet, "https://api.example.com/v1/users", nil)
res, err := client.Do(req)
```

```json
{"level":"WARN","msg":"http outbound","method":"GET","host":"api.example.com","path":"/v1/users","status":404,"latency":41000000,"size":120,"request_id":"abc123"}
```

- Info below 400, Warn for 4xx, Error for 5xx and transport errors.
- A nil `Logger` uses `slog.Default()` on each call, so after `slog.SetDefault(xlog.New(...))` the line follows xlog's format, service group and redaction. Context attributes such as `request_id` come from the request's context.
- The query string, headers and bodies are never logged: they often carry tokens and personal data.
- It is opt-in by use: code that never names `OutboundLog` does not link it, so there is no build tag.

## Struct tags

| Tag        | Where        | Meaning                                       |
| ---------- | ------------ | --------------------------------------------- |
| `json`     | body/response | property name, `-`, `omitempty`, `omitzero` |
| `doc`      | any field    | description                                   |
| `query`    | `Query` struct | query parameter name (same tag Fiber binds) |
| `required` | any field    | `"true"` / `"false"` overrides the default (query: optional; body: required unless `omitempty`/`omitzero`) |

## Build tags

| Tag | Effect |
| --- | --- |
| `xttp_slim` | Leaves the default middleware out of `ModeRaw` builds; an unset `Mode` panics. See [Mode](#mode). |
| `xttp_pprof` | Links pprof so `Profiling.Enabled` works. See [Profiling](#profiling). |

## Not included

- No validation beyond strict JSON decoding and required query parameters. Check field values in the handler and return an `xttp.Error`.
- No response encoding or handler wrapping. You call `c.JSON` yourself; `Responses` only documents what you send.
- No authentication beyond static bearer tokens and API keys. For JWT or OAuth, write a middleware that calls `SetPrincipal`.
- No way around Fiber's own weight. Fiber links its compression, TLS and `net/http` code whatever you use; `xttp` adds about 0.2 MB on top in a slim `ModeRaw` build.

## License

Apache 2.0, see [LICENSE](../../LICENSE)
