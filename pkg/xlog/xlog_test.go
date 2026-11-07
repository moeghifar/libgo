package xlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"testing/slogtest"
	"time"
)

func TestWithAttrs(t *testing.T) {
	ctx := WithAttrs(context.Background(), slog.String("request_id", "r1"))
	ctx2 := WithAttrs(ctx, slog.String("job", "j1"))
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"none", context.Background(), ""},
		{"nil ctx", nil, ""},
		{"one", ctx, `"request_id":"r1"`},
		{"appended", ctx2, `"request_id":"r1","job":"j1"`},
		{"parent unchanged", ctx, `"request_id":"r1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(Handler{Next: slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey {
						return slog.Attr{}
					}
					return a
				},
			})})
			if tt.ctx == nil {
				log.Info("hi") // slog substitutes context.Background()
			} else {
				log.InfoContext(tt.ctx, "hi")
			}
			if got := buf.String(); !strings.Contains(got, tt.want) {
				t.Fatalf("got %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestHandlerSlogtest(t *testing.T) {
	var buf bytes.Buffer
	h := Handler{Next: slog.NewJSONHandler(&buf, nil)}
	results := func() []map[string]any {
		var ms []map[string]any
		for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte{'\n'}) {
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatal(err)
			}
			ms = append(ms, m)
		}
		return ms
	}
	if err := slogtest.TestHandler(h, results); err != nil {
		t.Fatal(err)
	}
}

func TestRedact(t *testing.T) {
	r := Redact("password", "Token")
	tests := []struct {
		name string
		in   slog.Attr
		want slog.Attr
	}{
		{"match", slog.String("password", "x"), slog.String("password", Redacted)},
		{"case-insensitive", slog.String("TOKEN", "x"), slog.String("TOKEN", Redacted)},
		{"other", slog.String("user", "x"), slog.String("user", "x")},
		{"int", slog.Int("token", 1), slog.String("token", Redacted)},
		{"group kept", slog.Group("password", slog.String("a", "b")), slog.Group("password", slog.String("a", "b"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r(nil, tt.in); !got.Equal(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
	// end to end, inside a group
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: Redact("password")}))
	log.Info("x", slog.Group("db", slog.String("password", "hunter2")))
	if s := buf.String(); strings.Contains(s, "hunter2") || !strings.Contains(s, Redacted) {
		t.Fatalf("got %q", s)
	}
}

func TestSecret(t *testing.T) {
	s := Secret("hunter2")
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("x", "pw", s, "any", slog.AnyValue(s))
	out := buf.String() + fmt.Sprintf("%v %s", s, s)
	if strings.Contains(out, "hunter2") || strings.Count(out, Redacted) != 4 {
		t.Fatalf("got %q", out)
	}
}

func TestDSN(t *testing.T) {
	tests := []struct{ in, want string }{
		{"postgres://u:p@h:5432/db?sslmode=disable", "postgres://u:xxxxx@h:5432/db?sslmode=disable"},
		{"mysql://u@h/db", "mysql://u@h/db"},
		{"redis://:secret@h:6379/0", "redis://:xxxxx@h:6379/0"},
		{"host=h user=u password=p", "host=h user=u password=p"},
		{"", ""},
		{"://bad", "://bad"},
	}
	for _, tt := range tests {
		if got := DSN(tt.in); got != tt.want {
			t.Errorf("DSN(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDevHandler(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 6e6, time.UTC)
	tests := []struct {
		name string
		opts *DevOptions
		log  func(l *slog.Logger)
		want string
	}{
		{"plain", nil, func(l *slog.Logger) { l.Info("hi", "k", "v", "n", 1) }, "03:04:05.006 INFO  hi k=v n=1\n"},
		{"quoted", nil, func(l *slog.Logger) { l.Warn("a b", "k", "x y", "e", "") }, `03:04:05.006 WARN  a b k="x y" e=""` + "\n"},
		{"group", nil, func(l *slog.Logger) {
			l.WithGroup("g").With("a", 1).Error("m", slog.Group("h", "b", 2), "err", errors.New("boom"))
		},
			"03:04:05.006 ERROR m g.a=1 g.h.b=2 g.err=boom\n"},
		{"empty group dropped", nil, func(l *slog.Logger) { l.Info("m", slog.Group("g")) }, "03:04:05.006 INFO  m\n"},
		{"level filter", &DevOptions{Level: slog.LevelWarn}, func(l *slog.Logger) { l.Info("m") }, ""},
		{"debug", &DevOptions{Level: slog.LevelDebug}, func(l *slog.Logger) { l.Debug("m") }, "03:04:05.006 DEBUG m\n"},
		{"redact", &DevOptions{ReplaceAttr: Redact("pw")}, func(l *slog.Logger) { l.Info("m", "pw", "x") }, "03:04:05.006 INFO  m pw=[REDACTED]\n"},
		{"secret", nil, func(l *slog.Logger) { l.Info("m", "pw", Secret("x")) }, "03:04:05.006 INFO  m pw=[REDACTED]\n"},
		{"color", &DevOptions{Color: true}, func(l *slog.Logger) { l.Info("m", "k", "v") },
			"\x1b[2m03:04:05.006\x1b[0m \x1b[32mINFO \x1b[0m m \x1b[2mk=\x1b[0mv\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := NewDevHandler(&buf, tt.opts)
			l := slog.New(fixedTime{h, at})
			tt.log(l)
			if got := buf.String(); got != tt.want {
				t.Fatalf("got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestDevHandlerSource(t *testing.T) {
	var buf bytes.Buffer
	slog.New(NewDevHandler(&buf, &DevOptions{AddSource: true})).Info("m")
	if !strings.Contains(buf.String(), "m xlog_test.go:") {
		t.Fatalf("got %q", buf.String())
	}
}

func TestDevHandlerSlogtest(t *testing.T) {
	var buf bytes.Buffer
	h := NewDevHandler(&buf, &DevOptions{Level: slog.LevelDebug})
	results := func() []map[string]any {
		var ms []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			ms = append(ms, parseDevLine(line))
		}
		return ms
	}
	if err := slogtest.TestHandler(h, results); err != nil {
		t.Fatal(err)
	}
}

// parseDevLine turns "TIME LEVEL msg k=v g.k=v" into the nested map slogtest expects.
func parseDevLine(line string) map[string]any {
	m := map[string]any{}
	f := strings.Fields(line)
	if len(f) > 0 && len(f[0]) == 12 && f[0][2] == ':' {
		m[slog.TimeKey] = f[0]
		f = f[1:]
	}
	m[slog.LevelKey], m[slog.MessageKey] = f[0], f[1]
	for _, kv := range f[2:] {
		k, v, _ := strings.Cut(kv, "=")
		cur := m
		parts := strings.Split(k, ".")
		for _, g := range parts[:len(parts)-1] {
			sub, ok := cur[g].(map[string]any)
			if !ok {
				sub = map[string]any{}
				cur[g] = sub
			}
			cur = sub
		}
		cur[parts[len(parts)-1]] = v
	}
	return m
}

// fixedTime pins the record time so expected lines are stable.
type fixedTime struct {
	slog.Handler
	at time.Time
}

func (f fixedTime) Handle(ctx context.Context, r slog.Record) error {
	r.Time = f.at
	return f.Handler.Handle(ctx, r)
}

func (f fixedTime) WithAttrs(a []slog.Attr) slog.Handler {
	return fixedTime{f.Handler.WithAttrs(a), f.at}
}
func (f fixedTime) WithGroup(n string) slog.Handler { return fixedTime{f.Handler.WithGroup(n), f.at} }
