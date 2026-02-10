# climd

`climd` is a lightweight command-line interface builder designed as a simpler alternative to Cobra with minimal setup overhead. Optimized for speed and small binary size.

## Features

- Single array setup with struct initialization
- Support for flags: `./cmd <command> --param1 <val1> <val2> <val3> --param2 --param3 <val1>`
- Support for subcommands: `./cmd <command> <subcommand> [flags]`
- Context-aware execution
- Built-in help and version flags
- Minimal dependencies
- Small binary footprint

## Usage

```go
package main

import (
    "context"
    "fmt"
    "os"
    "strings"
    
    "github.com/moeghifar/libgo/pkg/climd"
)

func main() {
    config := climd.AppConfig{
        Name:        "myapp",
        Version:     "1.0.0",
        Description: "A sample CLI application",
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
        },
    }
    
    climd.Execute(config)
}
```

## Example Usage

```bash
# Serve command with flags
./cmd serve --http=all --grpc=module1 --consumer=module1,module2

# Exec command with boolean flags
./cmd exec --migrate-old-user --migrate-old-transactions

# DB command with subcommands
./cmd db init
./cmd db migrate
./cmd db create_sql --output=./migrations
```

## License

MIT