# xlog

What `log/slog` lacks: context attributes, redaction and a compact dev handler.

`xlog` adds three things to the standard `log/slog` and nothing else: attributes carried by a `context.Context` (so `request_id` appears on every line logged with that context), key-based redaction, and a one-line coloured handler for development. Standard library only, no global state, no `init()`.

```sh
go get github.com/moeghifar/libgo/pkg/xlog
```

## Features

- **One-call setup:** `New(Config{...})` builds a JSON or dev logger with level, source, redaction, a `message` key rename and a `service` group (name, version, env) computed once. It sets nothing global; you call `slog.SetDefault`.
- **Level functions:** `xlog.Debug`, `Info`, `Notice`, `Warn`, `Error`, `Fatal`, each `(ctx, msg, args...)`, logging on `slog.Default()` with the source pointing at your code.
- **Extra levels:** `LevelNotice` (between Info and Warn, for audit-style events) and `LevelFatal` (logs, then exits 1), printed as `NOTICE` and `FATAL`.
- **Context attributes:** `WithAttrs(ctx, ...)` plus `Handler{Next}` adds them to every `slog.InfoContext(ctx, ...)` line, always at the top level so `request_id` is found under the same key even inside groups.
- **Redaction:** `Redact("password", "authorization")` is a `ReplaceAttr` that masks matching keys case-insensitively, in any group. A group with a matching name is masked entirely.
- **Dev handler:** `NewDevHandler` writes one line per record with optional ANSI colour, `key=value` attributes, `group.key` names and `file:line` source.
- **Correct handlers:** both handlers pass `testing/slogtest`.

## Usage

```go
func main() {
	slog.SetDefault(xlog.New(xlog.Config{
		Service: "users", Version: "1.4.0", Env: "prod",
		Level:   slog.LevelInfo,
		Redact:  []string{"password", "authorization"},
	}))

	ctx := xlog.WithAttrs(context.Background(), slog.String("request_id", "abc123"))
	xlog.Info(ctx, "user created", "user_id", 42)
	xlog.Notice(ctx, "role changed", "user_id", 42, "role", "admin")
}
```

```json
{"time":"…","level":"INFO","msg":"user created","service":{"name":"users","version":"1.4.0","env":"prod"},"user_id":42,"request_id":"abc123"}
{"time":"…","level":"NOTICE","msg":"role changed","service":{…},"user_id":42,"role":"admin","request_id":"abc123"}
```

`slog.InfoContext(ctx, ...)` and a logger passed to `xttp` or `foreverwork` produce the same lines. Set `Dev: true` (and `Color`) for the one-line handler in development.

### Building the handler yourself

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/moeghifar/libgo/pkg/xlog"
)

func main() {
	redact := xlog.Redact("password", "authorization")

	var h slog.Handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: redact})
	if os.Getenv("LOG_FORMAT") == "dev" {
		st, _ := os.Stdout.Stat()
		h = xlog.NewDevHandler(os.Stdout, &xlog.DevOptions{
			Level:       slog.LevelDebug,
			Color:       st.Mode()&os.ModeCharDevice != 0, // colour only on a terminal
			AddSource:   true,
			ReplaceAttr: redact,
		})
	}
	slog.SetDefault(slog.New(xlog.Handler{Next: h}))

	ctx := xlog.WithAttrs(context.Background(), slog.String("request_id", "abc123"))
	slog.InfoContext(ctx, "login", "user", "ada", "password", "hunter2")
}
```

With `LOG_FORMAT=dev` this prints:

```
15:04:05.000 INFO  login main.go:27 user=ada password=[REDACTED] request_id=abc123
```

## Reference

| Symbol | Purpose |
| --- | --- |
| `New(Config) *slog.Logger` | JSON or dev logger with levels, redaction, context attrs and service group |
| `Config{Service, Version, Env, Level, Dev, Color, AddSource, Writer, MessageKey, Redact}` | Its options; the zero value is JSON on stdout at Info |
| `Debug`, `Info`, `Notice`, `Warn`, `Error`, `Fatal` `(ctx, msg, args...)` | Log on `slog.Default()`; a nil ctx is allowed. `Fatal` exits 1 without running deferred calls. |
| `LevelNotice`, `LevelFatal`, `LevelName(l)` | Extra levels (2 and 12) and their names |
| `WithAttrs(ctx, ...slog.Attr) context.Context` | Attach attributes to a context; repeated calls append |
| `Attrs(ctx) []slog.Attr` | Read them back |
| `Handler{Next slog.Handler}` | Adds the context attributes to each record, then calls `Next` |
| `Redact(keys ...string)` | `ReplaceAttr` function that masks matching keys |
| `Redacted` | The `"[REDACTED]"` replacement value |
| `NewDevHandler(w, *DevOptions) *DevHandler` | Compact one-line handler |
| `DevOptions{Level, Color, AddSource, ReplaceAttr}` | Its options. The zero value logs Info and above without colour or source. |

`DevOptions.Color` is a plain bool, so the decision stays at the call site (a TTY check, `NO_COLOR`, a flag) and the handler needs no `isatty` dependency.

## Not included

The standard library already has these, so `xlog` does not repeat them:

- Several outputs at once: `slog.NewMultiHandler`
- Changing the level at runtime: `slog.LevelVar`
- Discarding logs: `slog.DiscardHandler`

## License

Apache 2.0, see [LICENSE](../../LICENSE)
