# foreverwork

`foreverwork` runs long-lived worker pools: one pool per job type, each with its own goroutines, queue, per-job timeout and typed handler. No polling — a worker blocks on its channel and wakes the moment a job is queued. Standard library only.

## Features

- One typed pool per job type: `Submit` takes your struct, a wrong type is a compile error
- The goroutine count is the concurrency limit; the buffered queue is the backpressure
- A timeout on every job — zero means a default, never "no deadline"
- Panic recovery with the stack kept, an `OnPanic` hook, and a `*PanicError` you can `errors.As`
- Graceful shutdown with one budget: in-flight jobs keep a live context, then are cancelled; optional queue drain
- Cancellation causes (`ErrJobTimeout`, `ErrShutdown`) so a pool's ceiling is told apart from your own dependency's timeout
- `Every`: the other shape of long-lived work, a ticker that cannot overlap itself and survives a panic
- No global state: the logger is injected (`*slog.Logger`, default `slog.Default()`)

What it deliberately does not do: databases, batching, retries, deduplication. State that makes work resumable belongs in your tables, and a handler that finds its work already done returns without redoing it. A pool is not a durable queue.

## Usage

```go
package main

import (
    "context"
    "os/signal"
    "syscall"
    "time"

    "github.com/moeghifar/libgo/pkg/foreverwork"
)

type EmailJob struct{ To string }
type ReportJob struct{ ID int }

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    wp := foreverwork.New()
    wp.OnResult = func(r foreverwork.Result) { // optional: metrics, slow-job alerts
        // r.Pool, r.Job, r.Duration, r.Err, r.Cause
    }

    email := foreverwork.Register(wp,
        foreverwork.Config{Name: "email", Workers: 4, QueueSize: 100, Timeout: 10 * time.Second},
        func(ctx context.Context, j EmailJob) error { return send(ctx, j) })

    report := foreverwork.Register(wp,
        foreverwork.Config{Name: "report", Workers: 2, QueueSize: 20, Timeout: time.Minute},
        svc.Handle) // a method on your own struct works too
    report.Describe = func(j ReportJob) []any { return []any{"report_id", j.ID} }

    go foreverwork.Every(ctx, foreverwork.EveryConfig{
        Name: "sweep", Interval: 30 * time.Second, Timeout: 20 * time.Second,
    }, sweep)

    done := make(chan struct{})
    go func() { wp.Run(ctx); close(done) }()

    if err := email.Submit(ctx, EmailJob{To: "a@b.c"}); err != nil {
        // ctx.Err() if the caller gave up, foreverwork.ErrStopped if the pool is shutting down
    }

    <-ctx.Done() // SIGTERM
    <-done       // Run returns once jobs in flight are finished, or the grace ran out
}
```

Register every pool before `Run`; `Register` after `Run`, or a second `Run`, panics.

## Configuration

`Config` describes one pool:

| Field       | Meaning                                                                                      | Default            |
| ----------- | -------------------------------------------------------------------------------------------- | ------------------ |
| `Name`      | On every log line and in `Result`                                                            | `"worker"`         |
| `Workers`   | Goroutines = jobs running at once                                                            | `1`                |
| `QueueSize` | Jobs allowed to wait. `Submit` blocks when full — keep it small, that blocking is the point | `1`                |
| `Timeout`   | Deadline for one job                                                                         | `DefaultTimeout` (5m) |

`WorkerPool` fields, set before `Run`:

| Field               | Meaning                                                                                  | Default                    |
| ------------------- | ---------------------------------------------------------------------------------------- | -------------------------- |
| `Logger`            | `*slog.Logger` for the pool's lines                                                      | `slog.Default()`           |
| `OnResult`          | Called after every job. Concurrent and synchronous; a panic in it is recovered           | none                       |
| `OnPanic`           | Called with each recovered handler panic, after it is logged                             | none                       |
| `DisableStackTrace` | Leave the stack out of the panic log line                                                | stack is logged            |
| `ShutdownGrace`     | The whole shutdown budget, see below                                                     | `DefaultShutdownGrace` (20s) |
| `DrainQueue`        | Finish queued jobs on shutdown instead of dropping them                                  | `false`                    |

`Worker[T].Describe` (optional) replaces `"job", job` in log lines, so a job carrying a large slice does not print it twice. `Result.Job` always carries the whole job.

`EveryConfig`: `Name`, `Interval` (default 1m), `Timeout` per pass (default `DefaultTimeout`), `Logger`, `OnPanic`.

## Shutdown

Cancel the context you gave `Run`. Then:

1. `Submit` returns `ErrStopped` immediately, including a `Submit` that was blocked on a full queue.
2. Jobs in flight keep an **uncancelled** context for `ShutdownGrace`, so a job that is nearly done can write its own outcome. Cancelling it at once would fail that write and leave the row mid-flight.
3. If the grace runs out, their context is cancelled with cause `ErrShutdown`. `Run` returns when they notice.
4. Queued jobs are dropped — or, with `DrainQueue`, finished inside the same budget. `Run` logs `dropped=N`.

Keep `ShutdownGrace` under your platform's kill deadline (Kubernetes: `terminationGracePeriodSeconds`, default 30s).

## Contexts and causes

A job's context has the pool's `Timeout` and the shutdown budget. Both say why they fired:

```go
func handle(ctx context.Context, j Job) error {
    // ...
    if errors.Is(context.Cause(ctx), foreverwork.ErrShutdown) {
        // save partial progress and give up
    }
}
```

`Result.Err` is what the handler returned; `Result.Cause` is why the context was cancelled (`ErrJobTimeout`, `ErrShutdown`, or nil). Log lines and metrics are classified by `Cause`, so a handler whose own database call timed out is a *failure*, not a pool timeout — the fix for each is different. A handler that returns nil is a success even if shutdown began while it ran.

## Panics

A panic in a handler is recovered: the job fails, the runner takes the next one, and `Result.Err` is a `*PanicError{Name, Job, Value, Stack}`.

```go
wp.OnPanic = func(ctx context.Context, p *foreverwork.PanicError) {
    sentry.CaptureException(p) // p.Stack holds the panicking frames
}
```

`OnPanic` and `OnResult` run under their own recover, so a bug in a hook cannot take the process down. Not caught, as with any `recover`: a panic in a goroutine your handler starts, and runtime fatals (concurrent map writes, out of memory).

## Rules for handlers

1. Pass `ctx` to every database query and HTTP call. Timeout and shutdown both work through it; a handler that ignores it keeps `Run` waiting until the grace ends.
2. Be idempotent. Dropped jobs must be re-derivable from your own state, and a source that redelivers (a message queue) will deliver twice.
3. Do not spawn goroutines that can panic without their own recover.

## License

MIT
