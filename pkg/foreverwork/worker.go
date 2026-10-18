package foreverwork

import (
	"context"
	"runtime/debug"
	"slices"
	"sync/atomic"
	"time"
)

// Worker is one pool: its own queue, goroutines and handler for jobs of type T.
type Worker[T any] struct {
	// Describe, when set, says what to log about a job in place of the whole
	// value. A job that carries a 250-element slice would otherwise print it
	// twice per job. Set it right after Register, before Run.
	Describe func(T) []any

	cfg     Config
	jobs    chan T
	handler Handler[T]
	pool    *WorkerPool
	lost    atomic.Int64 // taken off the queue during shutdown and not run
}

// Submit queues a job. It blocks while the queue is full, and returns ctx.Err()
// if ctx is done first, or ErrStopped once the pool has begun shutting down.
//
// Blocking is the backpressure: a producer that outruns the pool waits for it,
// where a producer that could not queue would have to decide what to do with
// the job, and every producer would decide differently.
//
// A job accepted in the instant shutdown begins is treated like any other
// queued job: dropped, or drained if DrainQueue is set.
func (w *Worker[T]) Submit(ctx context.Context, job T) error {
	stopped := w.pool.stopped()
	// Checked first so that a refusal is certain: with both cases ready below,
	// select would pick at random and sometimes accept a job nobody will run.
	select {
	case <-stopped:
		return ErrStopped
	default:
	}
	select {
	case w.jobs <- job:
		return nil
	case <-stopped:
		return ErrStopped
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Queued is how many jobs are waiting. It is what tells a pool that is merely
// busy from one that is not draining.
func (w *Worker[T]) Queued() int { return len(w.jobs) }

// Name is the pool's name, for a caller that logs about its own queue.
func (w *Worker[T]) Name() string { return w.cfg.Name }

func (w *Worker[T]) config() Config { return w.cfg }

func (w *Worker[T]) dropped() int { return len(w.jobs) + int(w.lost.Load()) }

// fields is what a log line says about a job.
func (w *Worker[T]) fields(job T, runnerID int) []any {
	f := []any{"pool", w.cfg.Name, "runner", runnerID}
	if w.Describe != nil {
		return append(f, w.Describe(job)...)
	}
	return append(f, "job", job)
}

// work is one goroutine: take a job, run it, repeat until told to stop.
func (w *Worker[T]) work(loop, jobs context.Context, runnerID int) {
	for {
		job, ok := w.next(loop, jobs)
		if !ok {
			w.pool.log.InfoContext(loop, "worker stopped", "pool", w.cfg.Name, "runner", runnerID)
			return
		}

		fields := w.fields(job, runnerID)
		w.pool.log.InfoContext(loop, "worker job started", append(slices.Clip(fields), "queued", w.Queued())...)
		w.pool.report(loop, w.run(jobs, job), fields, w.Queued())
	}
}

// next returns the next job to run, or false when this runner should stop.
//
// Shutdown wins over a queued job. Both channels can be ready at once and
// select picks between them at random, so without the checks here a stopping
// pool keeps starting jobs — and the last one decides how long shutdown takes.
// With DrainQueue the runner instead keeps taking what is queued, until the
// queue is empty or the grace has run out.
func (w *Worker[T]) next(loop, jobs context.Context) (job T, ok bool) {
	if loop.Err() == nil {
		select {
		case <-loop.Done():
		case job = <-w.jobs:
			if loop.Err() == nil {
				return job, true
			}
			// Shutdown began while this job was being taken.
			if w.pool.DrainQueue && jobs.Err() == nil {
				return job, true
			}
			w.lost.Add(1)
			return job, false
		}
	}

	if !w.pool.DrainQueue || jobs.Err() != nil {
		return job, false
	}
	select {
	case job = <-w.jobs:
		return job, true
	default:
		return job, false
	}
}

// run executes one job under the pool's Timeout and recovers a panic.
//
// Recovering matters more than it looks: without it one malformed document
// takes the process down and interrupts every other job in flight. Here it
// fails that job, names it, keeps the stack, and the runner takes the next one.
func (w *Worker[T]) run(jobs context.Context, job T) (res Result) {
	res.Pool = w.cfg.Name
	res.Job = job
	res.Timeout = w.cfg.Timeout
	start := time.Now()

	// Always bounded: withDefaults guarantees a Timeout, so no handler can hold
	// a runner for ever. The cause says whose ceiling it was.
	ctx, cancel := context.WithTimeoutCause(jobs, w.cfg.Timeout, ErrJobTimeout)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			res.Err = &PanicError{Name: w.cfg.Name, Job: job, Value: r, Stack: debug.Stack()}
		}
		// Read before the deferred cancel above runs, or every job would look
		// cancelled.
		if ctx.Err() != nil {
			res.Cause = context.Cause(ctx)
		}
		res.Duration = time.Since(start)
	}()

	res.Err = w.handler(ctx, job)
	return res
}
