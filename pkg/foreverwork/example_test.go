package foreverwork_test

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/moeghifar/libgo/pkg/foreverwork"
)

func Example() {
	type EmailJob struct{ To string }

	ctx, stop := context.WithCancel(context.Background())
	sent := make(chan string, 1)

	wp := foreverwork.New()
	wp.Logger = slog.New(slog.DiscardHandler)
	email := foreverwork.Register(wp,
		foreverwork.Config{Name: "email", Workers: 2, QueueSize: 10, Timeout: 10 * time.Second},
		func(ctx context.Context, j EmailJob) error {
			sent <- j.To
			return nil
		})

	done := make(chan struct{})
	go func() { wp.Run(ctx); close(done) }()

	if err := email.Submit(ctx, EmailJob{To: "a@b.c"}); err != nil {
		fmt.Println("submit:", err)
	}
	fmt.Println(<-sent)

	stop() // SIGTERM in a real service; see signal.NotifyContext
	<-done
	fmt.Println(email.Submit(context.Background(), EmailJob{}))
	// Output:
	// a@b.c
	// foreverwork: pool stopped
}

func ExampleWorkerPool_OnPanic() {
	ctx, stop := context.WithCancel(context.Background())
	seen := make(chan string, 1)

	wp := foreverwork.New()
	wp.Logger = slog.New(slog.DiscardHandler)
	wp.OnPanic = func(_ context.Context, p *foreverwork.PanicError) {
		seen <- fmt.Sprintf("%s panicked with %v (stack: %v)", p.Name, p.Value, len(p.Stack) > 0)
	}
	w := foreverwork.Register(wp, foreverwork.Config{Name: "parse"},
		func(context.Context, int) error { panic("malformed document") })

	done := make(chan struct{})
	go func() { wp.Run(ctx); close(done) }()

	_ = w.Submit(ctx, 1)
	fmt.Println(<-seen)
	stop()
	<-done
	// Output: parse panicked with malformed document (stack: true)
}

func ExampleEvery() {
	ctx, stop := context.WithCancel(context.Background())
	swept := make(chan struct{}, 1)

	done := make(chan struct{})
	go func() {
		foreverwork.Every(ctx, foreverwork.EveryConfig{
			Name:     "sweep",
			Interval: time.Millisecond,
			Timeout:  time.Second,
			Logger:   slog.New(slog.DiscardHandler),
		}, func(context.Context) error {
			select {
			case swept <- struct{}{}:
			default:
			}
			return nil
		})
		close(done)
	}()

	<-swept
	stop()
	<-done
	fmt.Println("swept")
	// Output: swept
}
