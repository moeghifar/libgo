package xlog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewAndLevelFuncs(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(New(Config{
		Service: "users", Env: "prod", Level: slog.LevelDebug, AddSource: true,
		Writer: &buf, MessageKey: "message", Redact: []string{"password"},
	}))
	ctx := WithAttrs(context.Background(), slog.String("request_id", "r1"))

	for _, tc := range []struct {
		name  string
		log   func()
		level string
	}{
		{"debug", func() { Debug(ctx, "m", "password", "x") }, "DEBUG"},
		{"info", func() { Info(ctx, "m", "password", "x") }, "INFO"},
		{"notice", func() { Notice(ctx, "m", "password", "x") }, "NOTICE"},
		{"warn", func() { Warn(ctx, "m", "password", "x") }, "WARN"},
		{"error", func() { Error(ctx, "m", "password", "x") }, "ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			tc.log()
			var got struct {
				Level, Message, Password, RequestID string
				Service                             map[string]string
				Source                              struct{ File string }
			}
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("%v: %s", err, buf.String())
			}
			// field names: request_id needs a manual check
			if got.Level != tc.level || got.Message != "m" || got.Password != Redacted ||
				!strings.Contains(buf.String(), `"request_id":"r1"`) ||
				got.Service["name"] != "users" || got.Service["env"] != "prod" || got.Service["version"] != "" ||
				!strings.HasSuffix(got.Source.File, "logger_test.go") {
				t.Errorf("got %s", buf.String())
			}
		})
	}
}

func TestLevelFilterAndNilCtx(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(New(Config{Writer: &buf, Level: slog.LevelWarn}))
	Info(nil, "dropped") //nolint:staticcheck // nil ctx must not panic
	Notice(context.Background(), "dropped")
	if buf.Len() != 0 {
		t.Fatalf("below Warn logged: %s", buf.String())
	}
	Warn(nil, "kept") //nolint:staticcheck
	if !strings.Contains(buf.String(), `"msg":"kept"`) {
		t.Fatalf("got %s", buf.String())
	}
}

func TestDevLevelNames(t *testing.T) {
	var buf bytes.Buffer
	slog.New(NewDevHandler(&buf, nil)).Log(context.Background(), LevelNotice, "audit")
	if !strings.Contains(buf.String(), "NOTICE audit") {
		t.Fatalf("got %q", buf.String())
	}
}
