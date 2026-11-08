// Package xlog adds what log/slog does not ship: attributes carried by a
// context.Context, key-based redaction, and a compact coloured handler for
// development.
//
// It is stdlib only, has no global state and no init(). Multi-handlers, level
// switches and discard handlers are already in log/slog and are not repeated.
package xlog

import (
	"context"
	"log/slog"
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

// Handler wraps Next and adds the context attributes to each record, at the
// top level even when the logger has open groups, so a field like request_id
// is always found under the same key.
//
//	h := xlog.Handler{Next: slog.NewJSONHandler(os.Stdout, nil)}
//	slog.SetDefault(slog.New(h))
type Handler struct {
	Next slog.Handler

	root slog.Handler                      // Next as it was before the first group
	ops  []func(slog.Handler) slog.Handler // WithAttrs/WithGroup calls since root
}

// Enabled reports whether Next is enabled for level.
func (h Handler) Enabled(ctx context.Context, l slog.Level) bool { return h.Next.Enabled(ctx, l) }

// Handle adds the context attributes to r and forwards it to Next.
func (h Handler) Handle(ctx context.Context, r slog.Record) error {
	a := Attrs(ctx)
	if len(a) == 0 {
		return h.Next.Handle(ctx, r)
	}
	if h.root == nil {
		r = r.Clone()
		r.AddAttrs(a...)
		return h.Next.Handle(ctx, r)
	}
	// A group is open: put the attrs below it by replaying the chain.
	n := h.root.WithAttrs(a)
	for _, op := range h.ops {
		n = op(n)
	}
	return n.Handle(ctx, r)
}

// WithAttrs returns a Handler whose Next carries attrs.
func (h Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.with(h.root, func(n slog.Handler) slog.Handler { return n.WithAttrs(attrs) })
}

// WithGroup returns a Handler whose Next opens group name.
func (h Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	root := h.root
	if root == nil {
		root = h.Next
	}
	return h.with(root, func(n slog.Handler) slog.Handler { return n.WithGroup(name) })
}

func (h Handler) with(root slog.Handler, op func(slog.Handler) slog.Handler) slog.Handler {
	c := Handler{Next: op(h.Next), root: root}
	if root != nil {
		c.ops = append(slices.Clip(h.ops), op)
	}
	return c
}

// Redacted is the value every redacted attribute is replaced with.
const Redacted = "[REDACTED]"

// Redact returns a HandlerOptions.ReplaceAttr function that replaces the
// value of any attribute whose key matches one of keys, case-insensitively,
// in any group. Every attribute inside a group named by one of keys is
// replaced too.
func Redact(keys ...string) func(groups []string, a slog.Attr) slog.Attr {
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[strings.ToLower(k)] = struct{}{}
	}
	match := func(k string) bool {
		_, ok := set[strings.ToLower(k)]
		return ok
	}
	return func(groups []string, a slog.Attr) slog.Attr {
		if a.Value.Kind() != slog.KindGroup && (match(a.Key) || slices.ContainsFunc(groups, match)) {
			a.Value = slog.StringValue(Redacted)
		}
		return a
	}
}
