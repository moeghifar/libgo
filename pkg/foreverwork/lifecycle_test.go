package foreverwork

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mustPanic runs fn and fails unless it panics.
func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic", what)
		}
	}()
	fn()
}

// TestRegisterAfterRunPanics: the goroutines are started once, from the pools
// that exist then. A pool registered later would never run, and appending to
// the registry while Run reads it is a data race.
func TestRegisterAfterRunPanics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wp := New()
	wp.Logger = newLogger(&logBuf{})
	Register(wp, Config{Name: "a"}, func(context.Context, int) error { return nil })
	wait := run(t, wp, ctx)
	waitFor(t, "Run to start", wp.started.Load)

	mustPanic(t, "Register after Run", func() {
		Register(wp, Config{Name: "b"}, func(context.Context, int) error { return nil })
	})
	cancel()
	wait()
}

// TestSecondRunPanics: a second Run would double every pool's goroutines and
// so double the concurrency limit.
func TestSecondRunPanics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wp := New()
	wp.Logger = newLogger(&logBuf{})
	Register(wp, Config{Name: "a"}, func(context.Context, int) error { return nil })
	wait := run(t, wp, ctx)
	waitFor(t, "Run to start", wp.started.Load)

	mustPanic(t, "second Run", func() { wp.Run(ctx) })
	cancel()
	wait()
}

// TestSubmitAfterShutdownIsRefused: accepting a job into a queue nobody reads
// looks like success and loses the job.
func TestSubmitAfterShutdownIsRefused(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	wp := New()
	wp.Logger = newLogger(&logBuf{})
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 5}, func(context.Context, int) error { return nil })
	wait := run(t, wp, ctx)
	cancel()
	wait()

	if err := w.Submit(context.Background(), 1); !errors.Is(err, ErrStopped) {
		t.Errorf("Submit after shutdown = %v, want ErrStopped", err)
	}
	if w.Queued() != 0 {
		t.Errorf("Queued = %d: a refused job must not be enqueued", w.Queued())
	}
}

// TestABlockedSubmitIsReleasedByShutdown: a producer waiting on a full queue
// must not outlive the pool it is waiting for.
func TestABlockedSubmitIsReleasedByShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{}, 1)
	wp := New()
	wp.Logger = newLogger(&logBuf{})
	wp.ShutdownGrace = 30 * time.Millisecond
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1}, func(ctx context.Context, _ int) error {
		started <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	})
	wait := run(t, wp, ctx)

	_ = w.Submit(ctx, 1)
	<-started
	_ = w.Submit(ctx, 2) // fills the queue

	blocked := make(chan error, 1)
	go func() { blocked <- w.Submit(context.Background(), 3) }()
	time.Sleep(20 * time.Millisecond) // let it reach the full queue
	cancel()

	select {
	case err := <-blocked:
		if !errors.Is(err, ErrStopped) {
			t.Errorf("blocked Submit = %v, want ErrStopped", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the blocked Submit was never released")
	}
	wait()
}

// TestInFlightJobKeepsALiveContextDuringTheGrace: a job that is nearly done
// must be able to write its own outcome. On a cancelled context that write
// fails, and the row stays mid-flight until a watcher revives it.
func TestInFlightJobKeepsALiveContextDuringTheGrace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	proceed := make(chan struct{})
	var wrote atomic.Bool
	got := make(chan Result, 1)

	wp := New()
	wp.Logger = newLogger(&logBuf{})
	wp.ShutdownGrace = 5 * time.Second
	wp.OnResult = func(r Result) { got <- r }
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1}, func(ctx context.Context, _ int) error {
		close(started)
		<-proceed
		if ctx.Err() != nil { // the bookkeeping write, on the job's own context
			return ctx.Err()
		}
		wrote.Store(true)
		return nil
	})
	wait := run(t, wp, ctx)

	_ = w.Submit(ctx, 1)
	<-started
	cancel()
	time.Sleep(30 * time.Millisecond) // shutdown has begun; the grace has not run out
	close(proceed)
	wait()

	if !wrote.Load() {
		t.Error("the handler's context was cancelled as soon as shutdown began")
	}
	if r := <-got; r.Err != nil {
		t.Errorf("a job that finished inside the grace reported %v, want success", r.Err)
	}
}

// TestRunReturnsWithinTheGraceWhenAJobWillNotStop: the budget is the bound on
// shutdown. Without it one hung job holds the pod until it is killed.
func TestRunReturnsWithinTheGraceWhenAJobWillNotStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	wp := New()
	wp.Logger = newLogger(&logBuf{})
	wp.ShutdownGrace = 50 * time.Millisecond
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1, Timeout: time.Hour}, func(ctx context.Context, _ int) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	wait := run(t, wp, ctx)

	_ = w.Submit(ctx, 1)
	<-started
	begin := time.Now()
	cancel()
	wait()
	if took := time.Since(begin); took < 40*time.Millisecond || took > time.Second {
		t.Errorf("shutdown took %s, want about the 50ms grace", took)
	}
}

// TestATimeoutCarriesItsCause: the handler and the Result can both tell the
// pool's ceiling from anything else.
func TestATimeoutCarriesItsCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seen := make(chan error, 1)
	got := make(chan Result, 1)
	wp := New()
	wp.Logger = newLogger(&logBuf{})
	wp.OnResult = func(r Result) { got <- r }
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1, Timeout: 20 * time.Millisecond},
		func(ctx context.Context, _ int) error {
			<-ctx.Done()
			seen <- context.Cause(ctx)
			return ctx.Err()
		})
	wait := run(t, wp, ctx)

	_ = w.Submit(ctx, 1)
	if cause := <-seen; !errors.Is(cause, ErrJobTimeout) {
		t.Errorf("context.Cause inside the handler = %v, want ErrJobTimeout", cause)
	}
	if r := <-got; !errors.Is(r.Cause, ErrJobTimeout) || !errors.Is(r.Err, context.DeadlineExceeded) {
		t.Errorf("Result = {Err:%v Cause:%v}", r.Err, r.Cause)
	}
	cancel()
	wait()
}

// TestAHandlersOwnTimeoutIsAFailureNotAPoolTimeout: telling them apart by
// errors.Is(err, DeadlineExceeded) would blame the pool's ceiling for a slow
// database, and the fix for each is different.
func TestAHandlersOwnTimeoutIsAFailureNotAPoolTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buf := &logBuf{}
	got := make(chan Result, 1)
	wp := New()
	wp.Logger = newLogger(buf)
	wp.OnResult = func(r Result) { got <- r }
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1, Timeout: time.Hour},
		func(ctx context.Context, _ int) error {
			inner, stop := context.WithTimeout(ctx, 10*time.Millisecond)
			defer stop()
			<-inner.Done()
			return inner.Err() // DeadlineExceeded, from the handler's own dependency
		})
	wait := run(t, wp, ctx)

	_ = w.Submit(ctx, 1)
	r := <-got
	cancel()
	wait()

	if r.Cause != nil {
		t.Errorf("Cause = %v, want nil: the pool's context was never cancelled", r.Cause)
	}
	out := buf.String()
	if strings.Contains(out, "worker job timed out") || !strings.Contains(out, "worker job failed") {
		t.Errorf("a handler's own timeout was reported as the pool's ceiling:\n%s", out)
	}
}

// TestDrainQueueRunsQueuedJobs: with DrainQueue the work that was accepted is
// finished before the pool stops.
func TestDrainQueueRunsQueuedJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	buf := &logBuf{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var done atomic.Int64

	wp := New()
	wp.Logger = newLogger(buf)
	wp.DrainQueue = true
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 10}, func(_ context.Context, job int) error {
		if job == 0 {
			started <- struct{}{}
			<-release
		}
		done.Add(1)
		return nil
	})
	wait := run(t, wp, ctx)

	for i := range 3 {
		_ = w.Submit(ctx, i)
	}
	<-started
	cancel()
	close(release)
	wait()

	if done.Load() != 3 {
		t.Errorf("jobs completed = %d, want all 3 drained", done.Load())
	}
	if !strings.Contains(buf.String(), "dropped=0") {
		t.Errorf("shutdown line should report dropped=0:\n%s", buf.String())
	}
}

// TestDrainStopsWhenTheGraceRunsOut: draining is bounded by the same budget,
// and what it could not finish is counted, not silently lost.
func TestDrainStopsWhenTheGraceRunsOut(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	buf := &logBuf{}
	started := make(chan struct{})
	wp := New()
	wp.Logger = newLogger(buf)
	wp.DrainQueue = true
	wp.ShutdownGrace = 30 * time.Millisecond
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 10}, func(ctx context.Context, job int) error {
		if job == 0 {
			close(started)
		}
		<-ctx.Done()
		return ctx.Err()
	})
	wait := run(t, wp, ctx)

	for i := range 3 {
		_ = w.Submit(ctx, i)
	}
	<-started
	cancel()
	wait()

	if !strings.Contains(buf.String(), "dropped=2") {
		t.Errorf("shutdown line should report the 2 undrained jobs:\n%s", buf.String())
	}
}

// TestDroppedJobsAreCountedWithoutDrain: dropping is the default, so it has to
// be visible.
func TestDroppedJobsAreCountedWithoutDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	buf := &logBuf{}
	started := make(chan struct{})
	release := make(chan struct{})
	wp := New()
	wp.Logger = newLogger(buf)
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 10}, func(context.Context, int) error {
		close(started)
		<-release
		return nil
	})
	wait := run(t, wp, ctx)

	for i := range 3 {
		_ = w.Submit(ctx, i)
	}
	<-started
	cancel()
	time.Sleep(20 * time.Millisecond)
	close(release)
	wait()

	if !strings.Contains(buf.String(), "dropped=2") {
		t.Errorf("shutdown line should report dropped=2:\n%s", buf.String())
	}
}
