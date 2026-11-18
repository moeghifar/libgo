package xttp

import (
	"log/slog"
	"net/http"
	"time"
)

// OutboundLog is an http.RoundTripper that logs one line per outbound call:
// method, host, path, status, latency and response size. Wrap a client with it:
//
//	client := &http.Client{Transport: &xttp.OutboundLog{}}
//
// It logs with the request's context, so attributes stored with xlog.WithAttrs
// (request_id) land on the line when the logger is built with xlog.Handler.
// The query string and headers are never logged: they often carry secrets.
// Code that never names OutboundLog does not link it.
type OutboundLog struct {
	Next   http.RoundTripper // nil means http.DefaultTransport
	Logger *slog.Logger      // nil means slog.Default() at call time, so xlog.New set as default applies
}

// RoundTrip sends req through Next and logs the result: Info below 400, Warn
// for 4xx, Error for 5xx and transport errors.
func (t *OutboundLog) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}
	log := t.Logger
	if log == nil {
		log = slog.Default()
	}
	start := time.Now()
	res, err := next.RoundTrip(req)
	ctx := req.Context()

	level, status, size := slog.LevelInfo, 0, int64(-1)
	switch {
	case err != nil:
		level = slog.LevelError
	default:
		status, size = res.StatusCode, res.ContentLength
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}
	}
	if !log.Enabled(ctx, level) {
		return res, err
	}
	attrs := []slog.Attr{
		slog.String("method", req.Method),
		slog.String("host", req.URL.Host),
		slog.String("path", req.URL.Path),
		slog.Int("status", status),
		slog.Duration("latency", time.Since(start)),
		slog.Int64("size", size),
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	log.LogAttrs(ctx, level, "http outbound", attrs...)
	return res, err
}
