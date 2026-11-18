# libgo

Small, independent Go packages for building services. libgo is a set of tools, not a framework: take the one package you need and leave the rest.

```sh
go get github.com/moeghifar/libgo@latest
```

Requires Go 1.27.3 or later.

## Philosophy

libgo is built on a few rules. Every package follows them.

1. **Standard library first.** Any third-party dependency has to justify itself. Five of the six packages import only the standard library; `xttp` builds on Fiber because that is its job.
2. **Lightweight is the product.** Binary size, allocations and startup cost count as features. A feature that cannot pay for its weight does not ship, and anything optional can be compiled out with a build tag.
3. **Independent packages.** No package depends on another, except the stdlib-only `xerr` error model that they share. Importing `envy` does not pull in an HTTP server.
4. **Plain structs, not builders.** A struct literal describes the whole thing (a CLI, a server, a worker pool), so you can read your configuration in one place.
5. **Secure, production-ready defaults.** The zero value is the safe one: timeouts are on, payloads are kept out of logs, and debug endpoints are off. You opt out deliberately, never in by accident.
6. **No hidden state.** No package-level mutable variables, no `init()` side effects, and loggers are injected.
7. **Reflection at startup, never per request.** Struct tags are read once at registration; the hot path runs precomputed code.
8. **Errors are values.** Library code returns errors. It panics only on programmer mistakes detected at startup, such as a route registered without a handler.

## Packages

| Package | Purpose | Dependencies |
| --- | --- | --- |
| [`envy`](pkg/envy/README.md) | Load environment variables and an optional `.env` file into a tagged struct | godotenv, removable with `-tags libgo_envy_slim` |
| [`climd`](pkg/climd/README.md) | Command-line apps described by one struct; a small alternative to Cobra | stdlib |
| [`foreverwork`](pkg/foreverwork/README.md) | Typed worker pools and tickers with timeouts, panic recovery and graceful shutdown | stdlib |
| [`xerr`](pkg/xerr/README.md) | One typed error model (status, code, hidden cause) for HTTP, gRPC, workers and CLIs | stdlib |
| [`xlog`](pkg/xlog/README.md) | What `log/slog` lacks: context attributes, redaction, a compact dev handler | stdlib |
| [`xttp`](pkg/xttp/README.md) | Fiber v3 server with OpenAPI 3.1 generated from your types and hardened defaults | Fiber v3 |

Every package README has the same layout: install, features, usage, reference, build tags, and what the package deliberately leaves out.

## Build tags

Optional code stays out of your binary unless you ask for it.

| Tag | Effect |
| --- | --- |
| `libgo_envy_slim` | `envy` without `.env` support, so godotenv is not linked |
| `xttp_slim` | `xttp` without its default middleware when you use `ModeRaw` |
| `xttp_pprof` | Links pprof into `xttp`; without the tag, `Profiling.Enabled` panics at startup |

For release builds, also strip paths and debug info:

```sh
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" ./cmd/yourapp
```

## Example service

[`example/`](example) is a reference service to copy when starting a new one. `envy` loads the config, `climd` parses the command, and `serve` runs a hardened `xttp` API that queues jobs on a `foreverwork` pool.

```sh
go run ./example serve
```

## Development

```sh
make check   # runs every check below, in order
make test    # go test -race, with and without build tags
make lint    # golangci-lint
make vuln    # govulncheck
```

The `Makefile` also has `security`, `trivy`, `secrets` and `docker` targets. CI runs each check on every push and pull request. Releases follow conventional commits; see [CHANGELOG.md](CHANGELOG.md).

## License

[Apache 2.0](LICENSE)
