package main

import (
	"context"
	"log/slog"
)

// migrate is the slot for one-shot jobs. Open cfg.DSN with your driver here,
// and never log it.
func migrate(ctx context.Context, cfg MigrateConfig) error {
	if err := cfg.setup(); err != nil {
		return err
	}
	slog.InfoContext(ctx, "no migrations to run")
	return nil
}
