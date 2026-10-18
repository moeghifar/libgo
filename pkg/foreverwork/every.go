package foreverwork

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"
)

// EveryConfig describes one ticker.
type EveryConfig struct {
	// Name appears on every line the ticker logs and in a PanicError.
	Name string
	// Interval is the time between the end of one pass and the start of the
	// next tick. Zero or negative means one minute.
	Interval time.Duration
	// Timeout bounds one pass, for the same reason a pool bounds a job: a pass
	// that hangs on a query stops every later pass, and a reconciler that has
	// silently stopped reconciling is worse than one that logs a timeout every
	// interval. Zero means DefaultTimeout.
	Timeout time.Duration
	// Logger receives the ticker's lines. Nil means slog.Default(), resolved
	// when Every starts.
	Logger *slog.Logger
	// OnPanic, when set, is called with a recovered panic, after it has been
	// logged. It runs under its own recover.
	OnPanic func(context.Context, *PanicError)
}

// Every runs fn on a ticker until ctx is done.
//
// It is the other shape of long-lived work: a pool is for jobs that arrive, this
// is for a question asked on a schedule — sweep the queue, look for documents
// that stopped moving. Such a task needs no queue and no workers. It cannot
// overlap itself, because the next tick is only waited for once the pass has
// returned, so the "is one already running" bookkeeping a pool would need does
// not exist here.
//
// What it does keep from the pool is the part that matters: a panic in fn is
// recovered and the loop goes on. A bare `go func() { for range ticker.C ... }`
// takes the process down instead, and with it every job in flight.
//
// The caller decides whether to run it in a goroutine; Every blocks.
func Every(ctx context.Context, cfg EveryConfig, fn func(context.Context) error) {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	log := loggerOrDefault(cfg.Logger)

	log.InfoContext(ctx, "ticker starting",
		"ticker", cfg.Name, "every", cfg.Interval.String(), "timeout", cfg.Timeout.String())

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.InfoContext(ctx, "ticker stopped", "ticker", cfg.Name)
			return
		case <-ticker.C:
			runPass(ctx, cfg, log, fn)
		}
	}
}

// runPass is one pass, with its panic caught and its duration recorded.
//
// A slow pass delays the next tick rather than piling up: Go's ticker drops
// ticks that nobody is waiting for, which is what a periodic reconciler wants —
// two passes over the same rows would find the same work and do it twice.
func runPass(parent context.Context, cfg EveryConfig, log *slog.Logger, fn func(context.Context) error) {
	started := time.Now()
	name := cfg.Name

	log.DebugContext(parent, "ticker pass started", "ticker", name)

	ctx, cancel := context.WithTimeout(parent, cfg.Timeout)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			pe := &PanicError{Name: name, Value: r, Stack: debug.Stack()}
			log.ErrorContext(parent, "ticker pass panicked",
				"ticker", name, "duration_ms", time.Since(started).Milliseconds(),
				"error", pe, "stack", string(pe.Stack))
			if cfg.OnPanic != nil {
				safely(parent, log, "OnPanic", func() { cfg.OnPanic(parent, pe) })
			}
		}
	}()

	// The four endings a pass has, told apart for the same reason a job's are.
	// A cancellation is the pod stopping; a timeout is this pass being too slow
	// for its own ceiling, which the next tick will hit again.
	err := fn(ctx)
	fields := []any{"ticker", name, "duration_ms", time.Since(started).Milliseconds()}

	switch {
	case err == nil:
		log.DebugContext(parent, "ticker pass finished", fields...)
	case errors.Is(err, context.DeadlineExceeded) && parent.Err() == nil:
		log.ErrorContext(parent, "ticker pass timed out",
			append(fields, "timeout", cfg.Timeout.String(), "error", err)...)
	case parent.Err() != nil:
		log.WarnContext(parent, "ticker pass cancelled", append(fields, "error", err)...)
	default:
		log.ErrorContext(parent, "ticker pass failed", append(fields, "error", err)...)
	}
}

// loggerOrDefault resolves a nil logger to slog.Default() at call time, so the
// package holds no logger of its own.
func loggerOrDefault(l *slog.Logger) *slog.Logger {
	if l == nil {
		return slog.Default()
	}
	return l
}

// safely runs a caller-supplied hook and contains its panic. The recover in a
// pool exists so a bug cannot take the process down, and that has to hold for
// the hooks that observe the bug too.
func safely(ctx context.Context, log *slog.Logger, hook string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.ErrorContext(ctx, "worker hook panicked",
				"hook", hook, "error", fmt.Errorf("panic: %v", r), "stack", string(debug.Stack()))
		}
	}()
	fn()
}
