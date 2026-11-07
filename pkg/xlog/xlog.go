// Package xlog adds the three things log/slog does not ship: attributes
// carried by a context.Context, key- and type-based redaction, and a compact
// coloured handler for development.
//
// It is stdlib only, has no global state and no init(). Multi-handlers, level
// switches and discard handlers are already in log/slog and are not repeated.
package xlog

import (
	"context"
	"log/slog"
	"net/url"
	"slices"
	"strings"
)

// ctxKey is the context key for attributes added with WithAttrs.
type ctxKey struct{}

// WithAttrs returns a copy of ctx carrying attrs. A Handler appends them to
// every record logged with that context (slog.InfoContext and friends).
// Calling WithAttrs again appends to the existing attributes.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if len(attrs) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, append(slices.Clip(Attrs(ctx)), attrs...))
}

// Attrs returns the attributes stored in ctx by WithAttrs, or nil.
func Attrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	a, _ := ctx.Value(ctxKey{}).([]slog.Attr)
	return a
}

// Handler wraps Next and appends the context attributes to each record.
//
//	h := xlog.Handler{Next: slog.NewJSONHandler(os.Stdout, nil)}
//	slog.SetDefault(slog.New(h))
type Handler struct {
	Next slog.Handler
}

// Enabled reports whether Next is enabled for level.
func (h Handler) Enabled(ctx context.Context, l slog.Level) bool { return h.Next.Enabled(ctx, l) }

// Handle appends the context attributes to r and forwards it to Next.
func (h Handler) Handle(ctx context.Context, r slog.Record) error {
	if a := Attrs(ctx); len(a) > 0 {
		r = r.Clone()
		r.AddAttrs(a...)
	}
	return h.Next.Handle(ctx, r)
}

// WithAttrs returns a Handler whose Next carries attrs.
func (h Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return Handler{Next: h.Next.WithAttrs(attrs)}
}

// WithGroup returns a Handler whose Next opens group name.
func (h Handler) WithGroup(name string) slog.Handler { return Handler{Next: h.Next.WithGroup(name)} }

// Redacted is the value every redacted attribute is replaced with.
const Redacted = "[REDACTED]"

// Redact returns a HandlerOptions.ReplaceAttr function that replaces the
// value of any attribute whose key matches one of keys (case-insensitive,
// in any group) with Redacted.
func Redact(keys ...string) func(groups []string, a slog.Attr) slog.Attr {
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[strings.ToLower(k)] = struct{}{}
	}
	return func(_ []string, a slog.Attr) slog.Attr {
		if _, ok := set[strings.ToLower(a.Key)]; ok && a.Value.Kind() != slog.KindGroup {
			a.Value = slog.StringValue(Redacted)
		}
		return a
	}
}

// Secret is a string that logs as Redacted. Use it for passwords and tokens
// in config structs so they can never reach a log line by accident.
type Secret string

// LogValue implements slog.LogValuer.
func (Secret) LogValue() slog.Value { return slog.StringValue(Redacted) }

// String implements fmt.Stringer so %v and %s never print the secret either.
func (Secret) String() string { return Redacted }

// DSN masks the password in a URL-style DSN such as
// postgres://user:pass@host/db, returning postgres://user:xxxxx@host/db.
// A DSN without a password, or one that does not parse, is returned as is.
func DSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return dsn
	}
	if _, ok := u.User.Password(); !ok {
		return dsn
	}
	u.User = url.UserPassword(u.User.Username(), "xxxxx")
	return u.String()
}
