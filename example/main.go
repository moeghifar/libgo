package main

import (
	"fmt"

	"github.com/moeghifar/libgo/pkg/envy"
)

// Config represents the application configuration
type Config struct {
	App struct {
		Port int `env:"APP_PORT" default:"8080"`
	}
	Database struct {
		DSN  string `env:"DB_DSN"`
		Pool int    `env:"DB_POOL" default:"10"`
	}
	AllowedHosts []string `env:"ALLOWED_HOSTS" default:"localhost,127.0.0.1"`
	MustExist    int      `env:"MUST_EXIST" default:"999" required:"true"`
}

func main() {
	var cfg Config

	// Load configuration from environment variables and optional .env file
	if err := envy.Load(&cfg); err != nil {
		panic("Failed to load configuration: " + err.Error())
	}

	fmt.Printf("Loaded Configuration:\n")
	fmt.Printf("Port: %d\n", cfg.App.Port)
	fmt.Printf("DSN: %s\n", cfg.Database.DSN)
	fmt.Printf("Pool: %d\n", cfg.Database.Pool)
	fmt.Printf("Allowed Hosts: %v\n", cfg.AllowedHosts)
	fmt.Printf("Must Exist: %v\n", cfg.MustExist)
}
