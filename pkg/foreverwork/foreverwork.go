// Package foreverwork runs long-lived worker pools: one pool per job type,
// each with its own goroutines, queue, per-job timeout and handler.
//
//	wp := foreverwork.New()
//	triage := foreverwork.Register(wp, foreverwork.Config{Name: "triage", Workers: 2, QueueSize: 100}, handleTriage)
//	go wp.Run(ctx)            // returns once ctx is done and running jobs have finished
//	triage.Submit(ctx, job)   // typed, checked at compile time
//
// A module says what a job is and what to do with it, and nothing else.
// Queueing, the goroutines, the logging, panic recovery and the shutdown are
// here.
//
// What it deliberately does not know about: databases, batching, retries and
// deduplication. A pool that knew about those would have to know about the work
// too. The state that makes work resumable belongs in the caller's tables, and
// a handler that finds its work already done returns without doing it again.
//
// The package holds no global state: logging goes through the Logger on the
// pool (slog.Default() when it is nil), and every hook is a field on the value
// it belongs to.
package foreverwork

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Errors a handler or a producer can see.
var (
	// ErrJobTimeout is the cancellation cause of a job that ran past its pool's
	// Timeout. A handler reads it with context.Cause(ctx).
	ErrJobTimeout = errors.New("foreverwork: job exceeded its timeout")
	// ErrShutdown is the cancellation cause of a job still running when the
	// pool's shutdown grace period ran out.
	ErrShutdown = errors.New("foreverwork: pool shutting down")
	// ErrStopped is what Submit returns once shutdown has begun.
	ErrStopped = errors.New("foreverwork: pool stopped")
)

// Handler runs one job.
type Handler[T any] func(ctx context.Context, job T) error

// PanicError is a recovered panic. It is the Err of a job's Result, so
// errors.As finds it, and it is handed to OnPanic.
type PanicError struct {
	Name  string // pool or ticker name
	Job   any    // the job that panicked; nil for a ticker
	Value any    // what recover() returned
	Stack []byte // debug.Stack(), taken inside the recover, so it shows the panicking frames
}

// Error implements error.
func (p *PanicError) Error() string { return fmt.Sprintf("panic: %v", p.Value) }

// Unwrap returns the panic value when it is itself an error.
func (p *PanicError) Unwrap() error {
	err, _ := p.Value.(error)
	return err
}

// Result is reported after every job, for monitoring.
type Result struct {
	Pool     string
	Job      any // the job, so a failure can name what failed
	Duration time.Duration
	Timeout  time.Duration // the ceiling this job ran under, so a timeout says what it hit
	// Err is what the handler returned: nil on success, the handler's own error,
	// context.DeadlineExceeded on timeout, context.Canceled on shutdown, or a
	// *PanicError on a recovered panic.
	Err error
	// Cause is why the job's context was cancelled: ErrJobTimeout, ErrShutdown,
	// or nil when it never was. It, and not the text of Err, is what tells a
	// pool's ceiling from a timeout inside the handler's own dependency.
	Cause error
}

// Register adds a pool for jobs of type T. Call it before Run: the goroutines
// are started once, from the set of pools registered at that moment.
//
// Register panics if the pool is already running.
func Register[T any](wp *WorkerPool, cfg Config, h Handler[T]) *Worker[T] {
	if wp.started.Load() {
		panic("foreverwork: Register called after Run")
	}
	cfg = cfg.withDefaults()
	w := &Worker[T]{cfg: cfg, jobs: make(chan T, cfg.QueueSize), handler: h, pool: wp}
	wp.workers = append(wp.workers, w)
	return w
}
