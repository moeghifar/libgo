// Command example is the reference bootstrap for a libgo service: copy this
// directory to start a new one. See .env.example for its settings.
//
//	go run ./example serve
//	go run ./example migrate
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/moeghifar/libgo/pkg/climd"
	"github.com/moeghifar/libgo/pkg/envy"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

// run wires every command: config for that command, JSON logs, and a context
// cancelled on SIGINT or SIGTERM. Each command loads only what it needs, so
// migrate works without server settings and serve without a database.
func run[C any](fn func(context.Context, C) error) func(context.Context, []string, map[string]string) error {
	return func(ctx context.Context, _ []string, _ map[string]string) error {
		var cfg C
		if err := envy.Load(&cfg); err != nil {
			return fmt.Errorf("config: %w", err)
		}
		ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
		return fn(ctx, cfg)
	}
}

func main() {
	climd.Execute(climd.AppConfig{
		Name:        "example",
		Version:     version,
		Description: "Reference libgo service",
		Commands: []climd.Command{
			{Name: "serve", Short: "Run the HTTP API and workers", Run: run(serve)},
			{Name: "migrate", Short: "Run database migrations", Run: run(migrate)},
		},
	})
}
