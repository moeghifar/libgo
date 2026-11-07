package xlog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// DevOptions configures NewDevHandler. The zero value logs Info and above
// without colour or source.
type DevOptions struct {
	// Level is the minimum level to log; nil means slog.LevelInfo.
	Level slog.Leveler
	// Color enables ANSI colours. Decide at the call site, for example with
	// os.Stdout.Stat() and os.ModeCharDevice, so the handler needs no isatty.
	Color bool
	// AddSource appends file:line of the caller.
	AddSource bool
	// ReplaceAttr is applied to every attribute, as in slog.HandlerOptions.
	ReplaceAttr func(groups []string, a slog.Attr) slog.Attr
}

// DevHandler writes one human-readable line per record:
//
//	15:04:05.000 INFO  request done method=GET path=/users status=200
type DevHandler struct {
	opts   DevOptions
	w      io.Writer
	mu     *sync.Mutex
	groups []string
	prefix []byte // pre-rendered attrs from WithAttrs
}

// NewDevHandler returns a DevHandler writing to w. A nil opts uses defaults.
func NewDevHandler(w io.Writer, opts *DevOptions) *DevHandler {
	h := &DevHandler{w: w, mu: &sync.Mutex{}}
	if opts != nil {
		h.opts = *opts
	}
	return h
}

// Enabled reports whether l is at or above the configured level.
func (h *DevHandler) Enabled(_ context.Context, l slog.Level) bool {
	min := slog.LevelInfo
	if h.opts.Level != nil {
		min = h.opts.Level.Level()
	}
	return l >= min
}

// WithAttrs returns a handler with attrs pre-rendered into every line.
func (h *DevHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.prefix = bytes.Clone(h.prefix)
	for _, a := range attrs {
		c.prefix = c.appendAttr(c.prefix, a)
	}
	return &c
}

// WithGroup returns a handler that prefixes later attribute keys with name.
func (h *DevHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.groups = append(append([]string(nil), h.groups...), name)
	return &c
}

const (
	ansiReset = "\x1b[0m"
	ansiDim   = "\x1b[2m"
)

func levelColor(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "\x1b[31m" // red
	case l >= slog.LevelWarn:
		return "\x1b[33m" // yellow
	case l >= slog.LevelInfo:
		return "\x1b[32m" // green
	}
	return "\x1b[36m" // cyan
}

// Handle formats r as one line and writes it under the handler's lock.
func (h *DevHandler) Handle(_ context.Context, r slog.Record) error {
	var b []byte
	if !r.Time.IsZero() {
		if h.opts.Color {
			b = append(b, ansiDim...)
		}
		b = r.Time.AppendFormat(b, "15:04:05.000")
		if h.opts.Color {
			b = append(b, ansiReset...)
		}
		b = append(b, ' ')
	}
	if h.opts.Color {
		b = append(b, levelColor(r.Level)...)
	}
	b = fmt.Appendf(b, "%-5s", r.Level.String())
	if h.opts.Color {
		b = append(b, ansiReset...)
	}
	b = append(b, ' ')
	b = append(b, r.Message...)
	if h.opts.AddSource && r.PC != 0 {
		if s := r.Source(); s != nil {
			b = fmt.Appendf(b, " %s:%d", filepath.Base(s.File), s.Line)
		}
	}
	b = append(b, h.prefix...)
	r.Attrs(func(a slog.Attr) bool {
		b = h.appendAttr(b, a)
		return true
	})
	b = append(b, '\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(b)
	return err
}

// appendAttr renders " key=value", expanding groups as "g.key" and dropping
// empty attributes and empty groups.
func (h *DevHandler) appendAttr(b []byte, a slog.Attr) []byte {
	a.Value = a.Value.Resolve()
	if h.opts.ReplaceAttr != nil && a.Value.Kind() != slog.KindGroup {
		a = h.opts.ReplaceAttr(h.groups, a)
		a.Value = a.Value.Resolve()
	}
	if a.Equal(slog.Attr{}) {
		return b
	}
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		if len(attrs) == 0 {
			return b
		}
		g := h
		if a.Key != "" {
			c := *h
			c.groups = append(append([]string(nil), h.groups...), a.Key)
			g = &c
		}
		for _, ga := range attrs {
			b = g.appendAttr(b, ga)
		}
		return b
	}
	b = append(b, ' ')
	if h.opts.Color {
		b = append(b, ansiDim...)
	}
	for _, g := range h.groups {
		b = append(append(b, g...), '.')
	}
	b = append(b, a.Key...)
	b = append(b, '=')
	if h.opts.Color {
		b = append(b, ansiReset...)
	}
	return appendValue(b, a.Value)
}

func appendValue(b []byte, v slog.Value) []byte {
	switch v.Kind() {
	case slog.KindString:
		return appendString(b, v.String())
	case slog.KindTime:
		return v.Time().AppendFormat(b, time.RFC3339Nano)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return appendString(b, err.Error())
		}
		return appendString(b, fmt.Sprint(v.Any()))
	}
	return append(b, v.String()...)
}

func appendString(b []byte, s string) []byte {
	if s == "" || bytes.ContainsAny([]byte(s), " \t\n\"=") {
		return strconv.AppendQuote(b, s)
	}
	return append(b, s...)
}
