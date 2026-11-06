package main

import (
	"fmt"
	"log/slog"
	"os"
)

// Log is shared by every command.
type Log struct {
	Level string `env:"LOG_LEVEL" default:"INFO"`
}

// setup makes JSON on stdout the default logger.
func (l Log) setup() error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(l.Level)); err != nil {
		return fmt.Errorf("LOG_LEVEL: %w", err)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	return nil
}

// ServeConfig is the serve command's environment.
type ServeConfig struct {
	Log
	APITokens  []string `env:"API_TOKENS" required:"true"`
	Addr       string   `env:"APP_ADDR" default:"127.0.0.1:8080"`
	RateLimit  int      `env:"RATE_LIMIT" default:"100"`
	Docs       bool     `env:"DOCS_ENABLED" default:"false"`
	TrustProxy bool     `env:"TRUST_PROXY" default:"false"`
}

// MigrateConfig is the migrate command's environment.
type MigrateConfig struct {
	Log
	DSN string `env:"DB_DSN" required:"true"`
}
