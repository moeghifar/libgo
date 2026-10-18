package foreverwork

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// runner is one registered pool, whatever its job type.
type runner interface {
	config() Config
	// work is one goroutine. loop ends when shutdown begins; jobs is the context
	// handlers run on, which outlives loop by the shutdown grace.
	work(loop, jobs context.Context, runnerID int)
	// dropped is how many accepted jobs were never run.
	dropped() int
}

// WorkerPool owns every registered pool and their goroutines.
//
// Set its exported fields before Run. The workers read them while they run, so
// assigning one afterwards is a data race.
type WorkerPool struct {
	// OnResult, when set, is called after every job — success and failure
	// alike. It is where metrics go.
	//
	// It is called from the worker goroutines, concurrently and synchronously:
	// it must be safe for concurrent use, and a slow hook holds its runner. A
	// panic in it is recovered and logged.
	OnResult func(Result)
	// OnPanic, when set, is called with every recovered handler panic, after it
	// has been logged. It is the counterpart of fiber's StackTraceHandler and
	// runs under its own recover.
	OnPanic func(context.Context, *PanicError)
	// Logger receives the pool's lines. Nil means slog.Default(), resolved when
	// Run starts.
	Logger *slog.Logger
	// DisableStackTrace leaves the stack out of the log line for a panic. The
	// stack is on by default, unlike fiber's recover middleware: a panic here is
	// rare, and after it has been recovered the stack is the only clue left.
	DisableStackTrace bool
	// ShutdownGrace is the whole budget for shutdown. From the moment the
	// context given to Run is done, jobs in flight keep an uncancelled context
	// for this long, so a job that is nearly finished can write its own outcome.
	// When it runs out their context is cancelled with cause ErrShutdown and Run
	// returns once they notice. Zero means DefaultShutdownGrace.
	ShutdownGrace time.Duration
	// DrainQueue makes shutdown finish the jobs already queued, inside the same
	// budget, instead of dropping them. What is left when the budget runs out is
	// dropped and counted.
	DrainQueue bool

	workers  []runner
	log      *slog.Logger
	started  atomic.Bool
	initStop sync.Once
	stop     chan struct{} // closed when shutdown begins: Submit stops accepting
	stopOnce sync.Once
}

// New returns an empty pool. Register pools on it, then call Run.
func New() *WorkerPool { return &WorkerPool{} }

// stopped is closed once shutdown has begun.
func (wp *WorkerPool) stopped() <-chan struct{} {
	wp.initStop.Do(func() { wp.stop = make(chan struct{}) })
	return wp.stop
}

// beginShutdown closes the intake. It is safe to call from both the context's
// callback and Run's own return, whichever is first.
func (wp *WorkerPool) beginShutdown() {
	wp.stopped()
	wp.stopOnce.Do(func() { close(wp.stop) })
}

// Run starts every pool's goroutines and returns once ctx is done and the jobs
// in flight have finished, or the shutdown grace has run out and they have
// noticed.
//
// Once ctx is done Submit returns ErrStopped. Jobs still waiting in a queue are
// dropped unless DrainQueue is set, and the count is logged. That is safe only
// because what asks for the work is recorded elsewhere — in this service, a row
// in Postgres, which is swept again after a restart. A pool is not a durable
// queue.
//
// Run panics if it is called twice, or after Register: both are programmer
// errors that would otherwise double a pool's concurrency or start nothing.
func (wp *WorkerPool) Run(ctx context.Context) {
	if !wp.started.CompareAndSwap(false, true) {
		panic("foreverwork: Run called more than once")
	}
	wp.log = loggerOrDefault(wp.Logger)
	grace := wp.ShutdownGrace
	if grace <= 0 {
		grace = DefaultShutdownGrace
	}

	// Handlers run on jobs, not on ctx. It is cancelled only when the grace runs
	// out: cancelling with ctx would fail the very write a nearly finished job
	// is about to make.
	jobs, cancelJobs := context.WithCancelCause(context.WithoutCancel(ctx))
	defer cancelJobs(nil)

	finished := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		wp.beginShutdown()
		select {
		case <-time.After(grace):
			cancelJobs(ErrShutdown)
		case <-finished:
		}
	})
	defer stop()

	var wg sync.WaitGroup
	for _, w := range wp.workers {
		cfg := w.config()
		wp.log.InfoContext(ctx, "worker pool starting",
			"pool", cfg.Name, "workers", cfg.Workers, "queue_size", cfg.QueueSize,
			"timeout", cfg.Timeout.String())

		for id := range cfg.Workers {
			wg.Go(func() { w.work(ctx, jobs, id) })
		}
	}
	wg.Wait()
	close(finished)
	wp.beginShutdown()

	dropped := 0
	for _, w := range wp.workers {
		dropped += w.dropped()
	}
	wp.log.InfoContext(ctx, "worker pools stopped", "pools", len(wp.workers), "dropped", dropped)
}

// report logs how a job ended and hands the result to the hooks.
//
// The endings are told apart because they need different responses. A timeout
// means the ceiling is wrong or the dependency is hung; a cancellation means
// the pod is going away and is nobody's fault; a panic is a bug; a failure is
// the handler's own; and a finish is the line that proves throughput. Lumping
// them into "failed" makes a deploy look like an outage.
//
// A job is told apart by why its context was cancelled, not by what error it
// returned: a handler whose own database call timed out returns the same
// DeadlineExceeded as one that hit the pool's ceiling, and the two are fixed
// in different places.
//
// queued is the queue depth now, after the job, not the depth it started at.
func (wp *WorkerPool) report(ctx context.Context, r Result, fields []any, queued int) {
	fields = append(slices.Clip(fields), "queued", queued, "duration_ms", r.Duration.Milliseconds())

	var pe *PanicError
	switch {
	case r.Err == nil:
		wp.log.InfoContext(ctx, "worker job finished", fields...)
	case errors.As(r.Err, &pe):
		if !wp.DisableStackTrace {
			fields = append(fields, "stack", string(pe.Stack))
		}
		wp.log.ErrorContext(ctx, "worker job panicked", append(fields, "error", r.Err)...)
		if wp.OnPanic != nil {
			safely(ctx, wp.log, "OnPanic", func() { wp.OnPanic(ctx, pe) })
		}
	case errors.Is(r.Cause, ErrJobTimeout):
		wp.log.ErrorContext(ctx, "worker job timed out",
			append(fields, "timeout", r.Timeout.String(), "error", r.Err)...)
	case errors.Is(r.Cause, ErrShutdown):
		// Shutdown, not a fault: the pod was asked to stop and the grace ran
		// out while this job was in flight. Warn rather than Error so a deploy
		// is not an alert.
		wp.log.WarnContext(ctx, "worker job cancelled", append(fields, "error", r.Err)...)
	default:
		wp.log.ErrorContext(ctx, "worker job failed", append(fields, "error", r.Err)...)
	}

	if wp.OnResult != nil {
		safely(ctx, wp.log, "OnResult", func() { wp.OnResult(r) })
	}
}
