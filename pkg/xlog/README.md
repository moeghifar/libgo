# xlog

`xlog` adds the three things `log/slog` does not ship: attributes carried by a `context.Context`, key-based redaction, and a compact coloured handler for development. Stdlib only, no global state, no `init()`.

What it deliberately does **not** do (stdlib already has it): multi-handlers (`slog.NewMultiHandler`), dynamic levels (`slog.LevelVar`), discard (`slog.DiscardHandler`).

## Features

- `WithAttrs(ctx, attrs...)` + `Handler{Next}`: every `slog.InfoContext(ctx, ...)` carries `request_id`, `job`, etc.
- `Redact(keys...)` as `HandlerOptions.ReplaceAttr`: case-insensitive, works inside groups
- `NewDevHandler(w, &DevOptions{...})`: one line per record, optional ANSI colour, `key=value`, `g.key` for groups, `file:line` source
- Both handlers pass `testing/slogtest`

## Usage

```go
package main

import (
    "context"
    "log/slog"
    "os"

    "github.com/moeghifar/libgo/pkg/xlog"
)


func main() {
    var h slog.Handler
    if os.Getenv("LOG_FORMAT") == "dev" {
        st, _ := os.Stdout.Stat()
        h = xlog.NewDevHandler(os.Stdout, &xlog.DevOptions{
            Level:       slog.LevelDebug,
            Color:       st.Mode()&os.ModeCharDevice != 0, // colour only on a terminal
            AddSource:   true,
            ReplaceAttr: xlog.Redact("password", "authorization"),
        })
    } else {
        h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
            ReplaceAttr: xlog.Redact("password", "authorization"),
        })
    }
    log := slog.New(xlog.Handler{Next: h})

    ctx := xlog.WithAttrs(context.Background(), slog.String("request_id", "abc123"))
    // 15:04:05.000 INFO  connected main.go:31 dsn=postgres://app:xxxxx@db/app request_id=abc123

}
```

### Dev handler output

```
15:04:05.000 INFO  request done method=GET path=/users status=200 request_id=abc123
15:04:05.001 ERROR job failed job=email err="dial tcp: connection refused"
```

`DevOptions.Color` is a plain bool so the decision stays at the call site (TTY check, `NO_COLOR`, a flag). The handler has no `isatty` dependency.

## API

| Symbol | Purpose |
| --- | --- |
| `WithAttrs(ctx, ...slog.Attr) context.Context` | attach attrs to a context; repeated calls append |
| `Attrs(ctx) []slog.Attr` | read them back |
| `Handler{Next slog.Handler}` | appends context attrs to each record before `Next` |
| `Redact(keys ...string) func([]string, slog.Attr) slog.Attr` | `ReplaceAttr` that masks matching keys |
| `Redacted` | the `"[REDACTED]"` constant |
| `NewDevHandler(io.Writer, *DevOptions) *DevHandler` | compact one-line handler |
| `DevOptions{Level, Color, AddSource, ReplaceAttr}` | its options |
