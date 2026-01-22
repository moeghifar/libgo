# Envy

Simple, reflection-based environment variable loader for Go with optional `.env` file support using `godotenv`.

## Features

-   **Zero Config**: Just struct tags to define your config.
-   **Types**: Supports `string`, `int`, `uint`, `bool`, `float`, and `slice` types (`[]string`, `[]int`, etc.).
-   **Nested Structs**: Recursively parses nested structs for organized configuration.
-   **Optional .env**: Loads `.env` file if present (optional via build tags for production).
-   **Defaults & Required**: Struct tags for default values and required fields.

## Usage

### 1. Define your config struct

Use struct tags `env`, `default`, and `required`.

```go
type Config struct {
	AppPort int    `env:"APP_PORT" default:"8080"`
	Debug   bool   `env:"DEBUG" default:"false"`
	APIKey  string `env:"API_KEY" required:"true"`
	
	// Slices (comma separated in env)
	AllowedHosts []string `env:"ALLOWED_HOSTS" default:"localhost"`
	
	// Nested Structs
	Database struct {
		DSN string `env:"DB_DSN"`
	}
}
```

### 2. Load Configuration

```go
package main

import (
	"log"
	"github.com/moeghifar/libgo/pkg/envy"
)

func main() {
	var cfg Config
	
	// Loads from .env (if present) and environment variables
	if err := envy.Load(&cfg); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	
	// Usage
	log.Printf("Listening on port %d", cfg.AppPort)
}

```

## Build Optimization

By default, this package imports `github.com/joho/godotenv` to load `.env` files. This is great for development.

For production builds where you rely solely on system environment variables (e.g., Docker, Kubernetes) and want to save binary size (approx. 600KB), you can exclude the `.env` loading logic using the `libgo_envy_slim` build tag.

### Standard Build (Development)
Include `.env` support.

```bash
go build -o app main.go
```

### Efficient Build (Production)
Exclude `.env` support and reduce binary size.

```bash
go build -tags libgo_envy_slim -o app main.go
```
