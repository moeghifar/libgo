package xttp

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOutboundLog(t *testing.T) {
	for _, tc := range []struct {
		name string
		next rtFunc
		want []string
	}{
		{"ok", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, ContentLength: 5}, nil
		},
			[]string{"level=INFO", "status=200", "size=5", "host=api.test", "path=/v1/users"}},
		{"4xx", func(*http.Request) (*http.Response, error) { return &http.Response{StatusCode: 404}, nil }, []string{"level=WARN", "status=404"}},
		{"5xx", func(*http.Request) (*http.Response, error) { return &http.Response{StatusCode: 502}, nil }, []string{"level=ERROR", "status=502"}},
		{"err", func(*http.Request) (*http.Response, error) { return nil, errors.New("refused") }, []string{"level=ERROR", "status=0", "error=refused"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			c := &http.Client{Transport: &OutboundLog{Next: tc.next, Logger: slog.New(slog.NewTextHandler(&buf, nil))}}
			req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://api.test/v1/users?token=secret", nil)
			if res, err := c.Do(req); err == nil {
				_ = res.Body
			}
			out := buf.String()
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in %s", w, out)
				}
			}
			if strings.Contains(out, "secret") {
				t.Errorf("query leaked: %s", out)
			}
		})
	}
}
