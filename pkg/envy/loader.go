//go:build !libgo_envy_slim

package envy

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func loadEnvFile() error {
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			return fmt.Errorf("error loading .env file: %w", err)
		}
	}
	return nil
}
