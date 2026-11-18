package xlog

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func benchDefault(b *testing.B, l *slog.Logger, fn func(ctx context.Context)) {
	prev := slog.Default()
	b.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(l)
	ctx := WithAttrs(context.Background(), slog.String("request_id", "abc123"))
	b.ReportAllocs()
	for b.Loop() {
		fn(ctx)
	}
}

func BenchmarkSlogJSON(b *testing.B) {
	benchDefault(b, slog.New(slog.NewJSONHandler(io.Discard, nil)), func(ctx context.Context) {
		slog.InfoContext(ctx, "hello", "user_id", 42, "path", "/users")
	})
}

func BenchmarkXlogInfo(b *testing.B) {
	benchDefault(b, New(Config{Writer: io.Discard, Service: "users", Version: "1.0", Env: "prod", Redact: []string{"password"}}), func(ctx context.Context) {
		Info(ctx, "hello", "user_id", 42, "path", "/users")
	})
}

func BenchmarkXlogFiltered(b *testing.B) {
	benchDefault(b, New(Config{Writer: io.Discard}), func(ctx context.Context) {
		Debug(ctx, "hello", "user_id", 42)
	})
}

func BenchmarkDev(b *testing.B) {
	benchDefault(b, New(Config{Writer: io.Discard, Dev: true, Color: true}), func(ctx context.Context) {
		Info(ctx, "hello", "user_id", 42, "path", "/users")
	})
}

func BenchmarkSlogJSONFair(b *testing.B) {
	l := slog.New(slog.NewJSONHandler(io.Discard, nil)).With(slog.Group("service", "name", "users", "version", "1.0", "env", "prod"))
	benchDefault(b, l, func(ctx context.Context) {
		slog.InfoContext(ctx, "hello", "user_id", 42, "path", "/users", "request_id", "abc123")
	})
}

func BenchmarkSlogJSONReplace(b *testing.B) {
	r := func(_ []string, a slog.Attr) slog.Attr { return a }
	l := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{ReplaceAttr: r})).With(slog.Group("service", "name", "users", "version", "1.0", "env", "prod"))
	benchDefault(b, l, func(ctx context.Context) {
		slog.InfoContext(ctx, "hello", "user_id", 42, "path", "/users", "request_id", "abc123")
	})
}
