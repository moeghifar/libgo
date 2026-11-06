# libgo

A toolset of small Go libraries: lightweight, fast and clean. Each package under `pkg/` stands on its own, so you import only what you use, inside your own framework or directly.

```sh
go get github.com/moeghifar/libgo@latest
```

Requires Go 1.27.2 or later.

## Packages

| Package | What it does | Dependencies |
| --- | --- | --- |
| [`envy`](pkg/envy/README.md) | Loads environment variables and an optional `.env` file into a tagged struct | godotenv (drop it with `-tags libgo_envy_slim`) |
| [`climd`](pkg/climd/README.md) | Lightweight CLI builder, a Cobra alternative described by one struct | stdlib |
| [`foreverwork`](pkg/foreverwork/README.md) | Typed worker pools and tickers with timeouts, panic recovery and graceful shutdown | stdlib |
| [`xttp`](pkg/xttp/README.md) | Fiber v3 server with OpenAPI 3.1 generated from your structs, JSON:API errors and production defaults | Fiber v3 |

### xttp at a glance

```go
var ErrNotFound = xttp.Error{Status: 404, Code: "USR-404", Title: "User not found"}

api := xttp.New(xttp.Config{Info: xttp.Info{Title: "Users", Version: "1.0.0"}})
api.Add(xttp.Route{
	Method:    fiber.MethodGet,
	Path:      "/users/:id",
	Responses: map[int]any{200: User{}},
	Handler: func(c fiber.Ctx) error {
		u, err := repo.Find(c.Params("id"))
		if err != nil {
			return ErrNotFound.With("no user "+c.Params("id"), err) // err is logged, never sent
		}
		return c.JSON(u)
	},
})
api.Run(ctx, ":8080") // graceful stop on SIGINT/SIGTERM
```

Out of the box: request ID, slog access log, panic recovery, helmet headers and `Cache-Control: no-store` on every response (413/431 included), no `Server` banner, 100 req/s rate limit per IP, 1 MiB body limit, read/write/idle timeouts, dual-stack listener, Kubernetes probes (`/livez`, `/readyz`, `/startupz`), JSON:API errors including 404, and optional CORS, docs (`Docs{Enabled: true}`), pprof behind a guard, a `HelmetStrict` preset and strict `application/json` bodies. [`example/`](example) runs this hardened profile. See the [xttp README](pkg/xttp/README.md#server-defaults).

## Example

[`example/`](example) is the reference bootstrap: copy it to start a new service. `envy` loads each command's config, `climd` parses the command, and `serve` runs a hardened `xttp` API that queues jobs on a `foreverwork` pool. Settings are in [`.env.example`](example/.env.example), and business code goes in `internal/<module>`.

```sh
go run ./example serve
curl -X POST localhost:8080/v1/users -H 'Content-Type: application/json' \
  -d '{"name":"Ada","email":"ada@example.com"}'
open http://localhost:8080/docs
```

## Development

```sh
make check     # everything below, in order
make verify    # go mod verify
make vet       # go vet ./...
make test      # go test -race ./...
make lint      # golangci-lint: code quality
make security  # golangci-lint: security linters only
make vuln      # govulncheck: known vulnerabilities
make trivy     # trivy (Docker): dependencies, secrets, misconfiguration
make secrets   # gitleaks: secrets in git history
make docker    # build the hardened example image
make docker-run # run it isolated on 127.0.0.1:8080
```

CI runs each of these as its own step on every push and pull request.

Releases use [commit-and-tag-version](https://github.com/absolute-version/commit-and-tag-version) with conventional commits; see [CHANGELOG.md](CHANGELOG.md).

## License

[Apache 2.0](LICENSE)
