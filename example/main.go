package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/moeghifar/libgo/pkg/climd"
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

	appRunner := climd.AppConfig{
		Name:        "myapp",
		Version:     "1.0.0",
		Description: "A sample CLI application demonstrating climd features",
		Commands: []climd.Command{
			{
				Name:  "serve",
				Short: "Start the server with specified services",
				Long:  "This command starts the server with HTTP, gRPC, and consumer services",
				Flags: []climd.Flag{
					{
						Name:  "http",
						Usage: "HTTP service configuration (e.g., all, none, specific)",
					},
					{
						Name:  "grpc",
						Usage: "gRPC service modules (e.g., module1,module2)",
					},
					{
						Name:  "consumer",
						Usage: "Consumer service modules (e.g., module1,module2)",
					},
				},
				Run: func(ctx context.Context, args []string, flags map[string]string) error {
					http := flags["http"]
					grpc := flags["grpc"]
					consumer := flags["consumer"]

					fmt.Printf("Starting server...\n")
					fmt.Printf("HTTP: %s\n", http)
					fmt.Printf("gRPC: %s\n", grpc)
					fmt.Printf("Consumer: %s\n", consumer)

					return nil
				},
			},
			{
				Name:  "exec",
				Short: "Execute specific operations",
				Long:  "This command executes specific operations like migrations",
				Flags: []climd.Flag{
					{
						Name:  "migrate-old-user",
						Usage: "Migrate old user data",
					},
					{
						Name:  "migrate-old-transactions",
						Usage: "Migrate old transaction data",
					},
				},
				Run: func(ctx context.Context, args []string, flags map[string]string) error {
					if _, ok := flags["migrate-old-user"]; ok {
						fmt.Println("Migrating old user data...")
					}
					if _, ok := flags["migrate-old-transactions"]; ok {
						fmt.Println("Migrating old transaction data...")
					}

					return nil
				},
			},
			{
				Name:  "db",
				Short: "Database operations",
				Long:  "This command handles database operations like init, migrate, etc.",
				SubCommands: []climd.SubCommand{
					{
						Name:  "init",
						Short: "Initialize the database",
						Run: func(ctx context.Context, args []string, flags map[string]string) error {
							fmt.Println("Initializing database...")
							return nil
						},
					},
					{
						Name:  "migrate",
						Short: "Run database migrations",
						Run: func(ctx context.Context, args []string, flags map[string]string) error {
							fmt.Println("Running database migrations...")
							return nil
						},
					},
					{
						Name:  "create_sql",
						Short: "Generate SQL files",
						Flags: []climd.Flag{
							{
								Name:  "output",
								Short: "o",
								Usage: "Output directory for SQL files",
							},
						},
						Run: func(ctx context.Context, args []string, flags map[string]string) error {
							output := flags["output"]
							if output == "" {
								output = "./sql"
							}
							fmt.Printf("Creating SQL files in %s...\n", output)
							return nil
						},
					},
				},
			},
			{
				Name:  "config",
				Short: "Manage application configuration",
				Long:  "This command manages application configuration settings",
				Flags: []climd.Flag{
					{
						Name:  "set",
						Usage: "Set a configuration value (key=value format)",
					},
					{
						Name:  "get",
						Usage: "Get a configuration value by key",
					},
					{
						Name:  "remove",
						Usage: "Remove a configuration value by key",
					},
				},
				Run: func(ctx context.Context, args []string, flags map[string]string) error {
					if setValue, ok := flags["set"]; ok {
						parts := strings.SplitN(setValue, "=", 2)
						if len(parts) == 2 {
							fmt.Printf("Setting configuration: %s = %s\n", parts[0], parts[1])
						} else {
							fmt.Printf("Invalid format for --set. Use key=value\n")
						}
					}
					if getValue, ok := flags["get"]; ok {
						fmt.Printf("Getting configuration value for: %s\n", getValue)
					}
					if removeValue, ok := flags["remove"]; ok {
						fmt.Printf("Removing configuration value: %s\n", removeValue)
					}

					return nil
				},
			},
		},
	}

	climd.Execute(appRunner)
}
