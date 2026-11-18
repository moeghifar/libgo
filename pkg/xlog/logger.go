package xlog

import (
	"context"
	"io"
	"log/slog"
	"os"
	"runtime"
	"time"
)

// Extra levels, placed between slog's own so filtering by level still works:
// Notice sits between Info and Warn, Fatal above Error.
const (
	LevelNotice = slog.Level(2)
	LevelFatal  = slog.Level(12)
)

// LevelName is slog's level name, with NOTICE and FATAL for the extra levels.
func LevelName(l slog.Level) string {
	switch l {
	case LevelNotice:
		return "NOTICE"
	case LevelFatal:
		return "FATAL"
	}
	return l.String()
}

// Config describes the logger New builds. The zero value is a JSON logger on
// stdout at Info.
type Config struct {
	Service string // name, version and env go in a "service" group on every line,
	Version string // computed once here; empty ones are left out
	Env     string

	Level     slog.Leveler // minimum level; nil means Info. Use a *slog.LevelVar to change it at runtime
	Dev       bool         // one coloured line per record (NewDevHandler) instead of JSON
	Color     bool         // ANSI colours in Dev mode; decide at the call site (TTY, NO_COLOR)
	AddSource bool         // file:line of the caller
	Writer    io.Writer    // nil means os.Stdout

	MessageKey string   // renames the "msg" key, e.g. "message"; empty keeps "msg"
	Redact     []string // keys masked with Redacted, see Redact
}

// New builds a logger from cfg: JSON or dev output, the extra level names,
// redaction, context attributes (Handler) and the service group. It sets
// nothing global; call slog.SetDefault(xlog.New(cfg)) so the package-level
// functions and slog's own use it.
func New(cfg Config) *slog.Logger {
	w := cfg.Writer
	if w == nil {
		w = os.Stdout
	}
	var redact func([]string, slog.Attr) slog.Attr
	if len(cfg.Redact) > 0 {
		redact = Redact(cfg.Redact...)
	}
	replace := func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 {
			switch a.Key {
			case slog.LevelKey:
				if l, ok := a.Value.Any().(slog.Level); ok {
					a.Value = slog.StringValue(LevelName(l))
				}
				return a
			case slog.MessageKey:
				if cfg.MessageKey != "" {
					a.Key = cfg.MessageKey
				}
				return a
			}
		}
		if redact != nil {
			return redact(groups, a)
		}
		return a
	}

	var h slog.Handler
	if cfg.Dev {
		h = NewDevHandler(w, &DevOptions{Level: cfg.Level, Color: cfg.Color, AddSource: cfg.AddSource, ReplaceAttr: redact})
	} else {
		h = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: cfg.Level, AddSource: cfg.AddSource, ReplaceAttr: replace})
	}
	log := slog.New(Handler{Next: h})

	var svc []any
	for _, kv := range [][2]string{{"name", cfg.Service}, {"version", cfg.Version}, {"env", cfg.Env}} {
		if kv[1] != "" {
			svc = append(svc, slog.String(kv[0], kv[1]))
		}
	}
	if len(svc) > 0 {
		log = log.With(slog.Group("service", svc...))
	}
	return log
}

// Debug logs at slog.LevelDebug on slog.Default(). Attributes stored in ctx
// with WithAttrs are added when the default logger was built with Handler
// (New does that). The source is the caller of Debug.
func Debug(ctx context.Context, msg string, args ...any) { emit(ctx, slog.LevelDebug, msg, args) }

// Info logs at slog.LevelInfo on slog.Default().
func Info(ctx context.Context, msg string, args ...any) { emit(ctx, slog.LevelInfo, msg, args) }

// Notice logs at LevelNotice on slog.Default(): a normal but significant
// event, such as an audit entry.
func Notice(ctx context.Context, msg string, args ...any) { emit(ctx, LevelNotice, msg, args) }

// Warn logs at slog.LevelWarn on slog.Default().
func Warn(ctx context.Context, msg string, args ...any) { emit(ctx, slog.LevelWarn, msg, args) }

// Error logs at slog.LevelError on slog.Default().
func Error(ctx context.Context, msg string, args ...any) { emit(ctx, slog.LevelError, msg, args) }

// Fatal logs at LevelFatal on slog.Default() and exits with status 1.
// Deferred functions do not run; use it only in main.
func Fatal(ctx context.Context, msg string, args ...any) {
	emit(ctx, LevelFatal, msg, args)
	os.Exit(1)
}

// emit is slog.Logger.log with the caller frame of the exported function's
// caller, so AddSource points at user code rather than at xlog.
func emit(ctx context.Context, level slog.Level, msg string, args []any) {
	if ctx == nil {
		ctx = context.Background()
	}
	h := slog.Default().Handler()
	if !h.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // Callers, emit, Info
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = h.Handle(ctx, r)
}
