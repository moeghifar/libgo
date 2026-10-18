package foreverwork

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// waitFor polls until cond holds or the test runs out of patience. A pool is
// goroutines, so every assertion about it is eventual.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if cond() {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		case <-time.After(time.Millisecond):
		}
	}
}

// run starts the pool and returns a func that waits for it to stop.
func run(t *testing.T, wp *WorkerPool, ctx context.Context) func() {
	t.Helper()
	done := make(chan struct{})
	go func() { wp.Run(ctx); close(done) }()
	return func() {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Run did not return")
		}
	}
}

// TestWorkersRunAtMostConcurrency: the goroutine count is the limit, so a pool
// of two never has three jobs in flight however many are queued.
func TestWorkersRunAtMostConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var inFlight, peak atomic.Int64
	release := make(chan struct{})

	wp := New()
	w := Register(wp, Config{Name: "test", Workers: 2, QueueSize: 10},
		func(context.Context, int) error {
			n := inFlight.Add(1)
			for {
				if p := peak.Load(); n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			<-release
			inFlight.Add(-1)
			return nil
		})
	wait := run(t, wp, ctx)

	for i := range 6 {
		if err := w.Submit(ctx, i); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	waitFor(t, "both workers busy", func() bool { return inFlight.Load() == 2 })
	close(release)

	waitFor(t, "every job to finish", func() bool { return w.Queued() == 0 && inFlight.Load() == 0 })
	if got := peak.Load(); got != 2 {
		t.Errorf("peak jobs in flight = %d, want 2", got)
	}
	cancel()
	wait()
}

// TestSubmitWaitsForRoomAndHonoursContext: a full queue makes the producer
// wait. That is the backpressure; a producer that gave up quietly would be the
// bug.
func TestSubmitWaitsForRoomAndHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Not started: nothing drains, so the queue fills and stays full.
	wp := New()
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1}, func(context.Context, int) error { return nil })

	if err := w.Submit(ctx, 1); err != nil {
		t.Fatalf("first submit: %v", err)
	}

	short, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stop()
	if err := w.Submit(short, 2); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("submit into a full queue = %v, want the caller's deadline", err)
	}
}

// TestAPanickingHandlerIsReportedAndThePoolSurvives: one malformed document
// must not take the process down with every other job in flight.
func TestAPanickingHandlerIsReportedAndThePoolSurvives(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var results []Result
	var ok atomic.Int64

	wp := New()
	wp.OnResult = func(r Result) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	}
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 10}, func(_ context.Context, job int) error {
		if job == 0 {
			panic("bad document")
		}
		ok.Add(1)
		return nil
	})
	wait := run(t, wp, ctx)

	for _, job := range []int{0, 1, 2} {
		if err := w.Submit(ctx, job); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	waitFor(t, "the jobs after the panic", func() bool { return ok.Load() == 2 })

	mu.Lock()
	defer mu.Unlock()
	if len(results) != 3 || results[0].Err == nil {
		t.Fatalf("results = %+v, want the panic reported first", results)
	}
	if got := results[0].Err.Error(); got != "panic: bad document" {
		t.Errorf("panic error = %q", got)
	}
	cancel()
	wait()
}

// TestTimeoutCancelsTheJob: a pool that sets one bounds a handler that has no
// deadline of its own.
func TestTimeoutCancelsTheJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan error, 1)
	wp := New()
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1, Timeout: 20 * time.Millisecond},
		func(ctx context.Context, _ int) error {
			<-ctx.Done()
			return ctx.Err()
		})
	wp.OnResult = func(r Result) { got <- r.Err }
	wait := run(t, wp, ctx)

	if err := w.Submit(ctx, 1); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case err := <-got:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("job error = %v, want the deadline", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the job was never cut off")
	}
	cancel()
	wait()
}

// TestRunWaitsForRunningJobsAndDropsQueuedOnes: shutdown finishes what is in
// flight. What is still queued is dropped, which is safe only because the
// database says the work was asked for.
func TestRunWaitsForRunningJobsAndDropsQueuedOnes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	finished := make(chan struct{})
	var done atomic.Int64

	wp := New()
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 10}, func(context.Context, int) error {
		started <- struct{}{}
		<-finished
		done.Add(1)
		return nil
	})
	wait := run(t, wp, ctx)

	for i := range 3 {
		if err := w.Submit(ctx, i); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	<-started
	cancel()

	// Run must still be inside the running job.
	time.Sleep(20 * time.Millisecond)
	if done.Load() != 0 {
		t.Fatal("the running job finished before it was released")
	}
	close(finished)
	wait()

	if done.Load() != 1 {
		t.Errorf("jobs completed = %d, want the one in flight", done.Load())
	}
	if w.Queued() == 0 {
		t.Error("the queued jobs were drained; shutdown must drop them, not run them")
	}
}

// TestDefaults: a zero config is usable, and the queue is never unbuffered —
// an unbuffered queue turns every hand-off into a rendezvous with a worker.
func TestDefaults(t *testing.T) {
	wp := New()
	w := Register(wp, Config{}, func(context.Context, int) error { return nil })

	if w.cfg.Timeout != DefaultTimeout {
		t.Errorf("default timeout = %s, want %s: no pool may run unbounded", w.cfg.Timeout, DefaultTimeout)
	}
	if w.cfg.Name != "worker" || w.cfg.Workers != 1 || w.cfg.QueueSize != 1 {
		t.Errorf("defaults = %+v", w.cfg)
	}
	if err := w.Submit(context.Background(), 1); err != nil {
		t.Errorf("submit into a default pool: %v", err)
	}
}

// TestEveryKeepsTickingAfterAPanic: a periodic reconciler that dies takes the
// pod with it, and the pod is running everything else. One bad pass must cost
// one pass.
func TestEveryKeepsTickingAfterAPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var passes atomic.Int64
	done := make(chan struct{})
	go func() {
		Every(ctx, EveryConfig{Name: "test", Interval: time.Millisecond, Timeout: time.Minute}, func(context.Context) error {
			if passes.Add(1) == 1 {
				panic("first pass is bad")
			}
			return nil
		})
		close(done)
	}()

	waitFor(t, "passes after the panic", func() bool { return passes.Load() >= 3 })
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Every did not return when its context ended")
	}
}

// TestEveryReportsAFailedPassAndContinues: an error is the normal outcome of a
// database being briefly unreachable, not a reason to stop looking.
func TestEveryReportsAFailedPassAndContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var passes atomic.Int64
	go Every(ctx, EveryConfig{Name: "test", Interval: time.Millisecond, Timeout: time.Minute}, func(context.Context) error {
		passes.Add(1)
		return errors.New("database is away")
	})

	waitFor(t, "several failed passes", func() bool { return passes.Load() >= 3 })
}

// TestATimedOutJobSaysWhatCeilingItHit: a timeout is the one failure whose fix
// is a setting, so the Result carries the ceiling and not only the error.
func TestATimedOutJobSaysWhatCeilingItHit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan Result, 1)
	wp := New()
	wp.OnResult = func(r Result) { got <- r }
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1, Timeout: 20 * time.Millisecond},
		func(ctx context.Context, _ int) error {
			<-ctx.Done()
			return ctx.Err()
		})
	wait := run(t, wp, ctx)

	if err := w.Submit(ctx, 1); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case r := <-got:
		if !errors.Is(r.Err, context.DeadlineExceeded) {
			t.Errorf("error = %v, want the deadline", r.Err)
		}
		if r.Timeout != 20*time.Millisecond {
			t.Errorf("Timeout = %s, want the pool's 20ms", r.Timeout)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the job was never cut off")
	}
	cancel()
	wait()
}

// TestShutdownIsNotReportedAsAFailure: a cancelled job and a broken one need
// different responses, so they must not arrive as the same error. This is the
// distinction the log lines are built on. The job is cut off only when the
// shutdown grace period ends, and says why.
func TestShutdownIsNotReportedAsAFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	got := make(chan Result, 1)

	wp := New()
	wp.ShutdownGrace = 30 * time.Millisecond
	wp.OnResult = func(r Result) { got <- r }
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1}, func(ctx context.Context, _ int) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	wait := run(t, wp, ctx)

	if err := w.Submit(ctx, 1); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	<-started
	cancel()

	select {
	case r := <-got:
		if !errors.Is(r.Err, context.Canceled) || errors.Is(r.Err, context.DeadlineExceeded) {
			t.Errorf("error = %v, want a cancellation and not a timeout", r.Err)
		}
		if !errors.Is(r.Cause, ErrShutdown) {
			t.Errorf("Cause = %v, want ErrShutdown", r.Cause)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the cancelled job was never reported")
	}
	wait()
}

// TestATickerPassIsBounded: a pass that hangs on a query would stop every later
// pass, so the reconciler would quietly stop reconciling.
func TestATickerPassIsBounded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var timedOut atomic.Int64
	go Every(ctx, EveryConfig{Name: "test", Interval: time.Millisecond, Timeout: 20 * time.Millisecond}, func(ctx context.Context) error {
		<-ctx.Done()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			timedOut.Add(1)
		}
		return ctx.Err()
	})

	waitFor(t, "passes cut off by their own ceiling", func() bool { return timedOut.Load() >= 2 })
}
