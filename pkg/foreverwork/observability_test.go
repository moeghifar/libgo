package foreverwork

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// logBuf is a goroutine-safe sink for an injected logger.
type logBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func newLogger(buf *logBuf) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// TestAPanicIsAPanicErrorWithItsStack: the stack is the only diagnostic left
// once the process has been saved, so it must reach whoever handles the result.
func TestAPanicIsAPanicErrorWithItsStack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan Result, 1)
	wp := New()
	wp.Logger = newLogger(&logBuf{})
	wp.OnResult = func(r Result) { got <- r }
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1}, func(context.Context, int) error {
		panic("bad document")
	})
	wait := run(t, wp, ctx)

	if err := w.Submit(ctx, 7); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	r := <-got

	var pe *PanicError
	if !errors.As(r.Err, &pe) {
		t.Fatalf("Err = %v, want a *PanicError", r.Err)
	}
	if pe.Value != "bad document" || pe.Name != "test" || pe.Job != 7 {
		t.Errorf("PanicError = {Name:%q Job:%v Value:%v}", pe.Name, pe.Job, pe.Value)
	}
	if !bytes.Contains(pe.Stack, []byte("observability_test.go")) {
		t.Errorf("stack does not reach the panicking handler:\n%s", pe.Stack)
	}
	if r.Err.Error() != "panic: bad document" {
		t.Errorf("Error() = %q", r.Err.Error())
	}
	cancel()
	wait()
}

// TestOnPanicHookReceivesThePanic: the analogue of fiber's StackTraceHandler.
func TestOnPanicHookReceivesThePanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan *PanicError, 1)
	wp := New()
	wp.Logger = newLogger(&logBuf{})
	wp.OnPanic = func(_ context.Context, p *PanicError) { got <- p }
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1}, func(context.Context, int) error {
		panic("boom")
	})
	wait := run(t, wp, ctx)

	_ = w.Submit(ctx, 1)
	select {
	case p := <-got:
		if p.Value != "boom" || len(p.Stack) == 0 {
			t.Errorf("OnPanic got %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnPanic was never called")
	}
	cancel()
	wait()
}

// TestAPanickingHookDoesNotKillThePool: the recover exists so a bug cannot take
// the process down, and that has to hold for the hooks that observe the bug.
func TestAPanickingHookDoesNotKillThePool(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var ok atomic.Int64
	buf := &logBuf{}
	wp := New()
	wp.Logger = newLogger(buf)
	wp.OnResult = func(Result) { panic("metrics client exploded") }
	wp.OnPanic = func(context.Context, *PanicError) { panic("panic hook exploded") }
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 10}, func(_ context.Context, job int) error {
		if job == 0 {
			panic("bad document")
		}
		ok.Add(1)
		return nil
	})
	wait := run(t, wp, ctx)

	for _, job := range []int{0, 1, 2} {
		_ = w.Submit(ctx, job)
	}
	waitFor(t, "jobs after the hooks panicked", func() bool { return ok.Load() == 2 })

	if !strings.Contains(buf.String(), "worker hook panicked") {
		t.Errorf("a panicking hook was not logged:\n%s", buf.String())
	}
	cancel()
	wait()
}

// TestInjectedLoggerReceivesThePoolsLines: the pool logs through the logger it
// was given, not through a package global.
func TestInjectedLoggerReceivesThePoolsLines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buf := &logBuf{}
	var done atomic.Bool
	wp := New()
	wp.Logger = newLogger(buf)
	w := Register(wp, Config{Name: "triage", Workers: 1, QueueSize: 1}, func(context.Context, int) error {
		done.Store(true)
		return nil
	})
	wait := run(t, wp, ctx)

	_ = w.Submit(ctx, 1)
	waitFor(t, "the job", func() bool { return done.Load() })
	cancel()
	wait()

	out := buf.String()
	for _, want := range []string{"worker pool starting", "pool=triage", "worker job started", "worker job finished", "worker pools stopped"} {
		if !strings.Contains(out, want) {
			t.Errorf("log is missing %q:\n%s", want, out)
		}
	}
}

// TestStackTraceIsLoggedUnlessDisabled: fiber ships stack traces off; here the
// default is on, with a switch.
func TestStackTraceIsLoggedUnlessDisabled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		disable bool
		want    bool
	}{{"default", false, true}, {"disabled", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			buf := &logBuf{}
			got := make(chan struct{}, 1)
			wp := New()
			wp.Logger = newLogger(buf)
			wp.DisableStackTrace = tc.disable
			wp.OnResult = func(Result) { got <- struct{}{} }
			w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1}, func(context.Context, int) error {
				panic("boom")
			})
			wait := run(t, wp, ctx)

			_ = w.Submit(ctx, 1)
			<-got
			cancel()
			wait()

			out := buf.String()
			if !strings.Contains(out, "worker job panicked") {
				t.Fatalf("a panic must have its own log line:\n%s", out)
			}
			if has := strings.Contains(out, "stack="); has != tc.want {
				t.Errorf("stack logged = %v, want %v:\n%s", has, tc.want, out)
			}
		})
	}
}

type bigJob struct {
	DocID int
	Pages [250]int
}

// TestDescribeReplacesTheJobInLogs: logging a 250-element array twice per job
// is the reason the hook exists. Result.Job still carries the whole job.
func TestDescribeReplacesTheJobInLogs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buf := &logBuf{}
	got := make(chan Result, 1)
	wp := New()
	wp.Logger = newLogger(buf)
	wp.OnResult = func(r Result) { got <- r }
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 1}, func(context.Context, bigJob) error { return nil })
	w.Describe = func(j bigJob) []any { return []any{"doc_id", j.DocID, "pages", len(j.Pages)} }
	wait := run(t, wp, ctx)

	_ = w.Submit(ctx, bigJob{DocID: 42})
	r := <-got
	cancel()
	wait()

	out := buf.String()
	if !strings.Contains(out, "doc_id=42") || !strings.Contains(out, "pages=250") {
		t.Errorf("Describe fields are missing:\n%s", out)
	}
	if strings.Contains(out, "job=") {
		t.Errorf("the whole job was logged despite Describe:\n%s", out)
	}
	if j, ok := r.Job.(bigJob); !ok || j.DocID != 42 {
		t.Errorf("Result.Job = %#v, want the submitted job", r.Job)
	}
}

// TestFinishedLineReportsTheQueueAfterTheJob: the depth printed when a job ends
// must be the depth then, not the depth when it began.
func TestFinishedLineReportsTheQueueAfterTheJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buf := &logBuf{}
	started := make(chan struct{})
	release := make(chan struct{})
	got := make(chan struct{}, 2)
	wp := New()
	wp.Logger = newLogger(buf)
	wp.OnResult = func(Result) { got <- struct{}{} }
	w := Register(wp, Config{Name: "test", Workers: 1, QueueSize: 5}, func(_ context.Context, job int) error {
		if job == 1 {
			close(started)
			<-release
		}
		return nil
	})
	wait := run(t, wp, ctx)

	_ = w.Submit(ctx, 1)
	<-started // queue is empty at the start of job 1
	_ = w.Submit(ctx, 2)
	close(release)
	<-got
	<-got
	cancel()
	wait()

	var finished string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "worker job finished") {
			finished = line
			break
		}
	}
	if !strings.Contains(finished, "queued=1") {
		t.Errorf("first finished line = %q, want queued=1 (job 2 was waiting when job 1 ended)", finished)
	}
}

// TestEveryUsesItsLoggerAndOnPanic: a ticker gets the same two seams as a pool.
func TestEveryUsesItsLoggerAndOnPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buf := &logBuf{}
	got := make(chan *PanicError, 1)
	var passes atomic.Int64
	go Every(ctx, EveryConfig{
		Name: "sweep", Interval: time.Millisecond, Logger: newLogger(buf),
		OnPanic: func(_ context.Context, p *PanicError) {
			select {
			case got <- p:
			default:
			}
		},
	}, func(context.Context) error {
		if passes.Add(1) == 1 {
			panic("bad pass")
		}
		return nil
	})

	select {
	case p := <-got:
		if p.Name != "sweep" || p.Value != "bad pass" || len(p.Stack) == 0 {
			t.Errorf("OnPanic got %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnPanic was never called")
	}
	waitFor(t, "passes after the panic", func() bool { return passes.Load() >= 3 })
	if out := buf.String(); !strings.Contains(out, "ticker=sweep") || !strings.Contains(out, "ticker pass panicked") {
		t.Errorf("injected logger missed the ticker's lines:\n%s", out)
	}
}
