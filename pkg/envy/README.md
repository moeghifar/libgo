# envy

Load environment variables, and an optional `.env` file, into a tagged struct.

You describe your configuration as a Go struct, and `envy` fills it from struct tags: default values first, then the `.env` file, then the real environment. The only dependency is godotenv for `.env` files, and a build tag removes it for production.

```sh
go get github.com/moeghifar/libgo/pkg/envy
```

## Features

- **Struct tags only:** `env`, `default` and `required` describe each field. There is nothing else to configure.
- **Typed values:** `string`, signed and unsigned integers, `bool`, floats, and slices of these (comma-separated).
- **Nested structs:** grouped settings are filled recursively.
- **Clear precedence:** defaults, then `.env`, then the environment. A variable that is already set always wins over `.env`.
- **Required fields:** a `required:"true"` field that ends up empty is an error. If it has a default, `envy` uses it and logs a warning instead.
- **Defaults only:** `Defaults` applies the `default` tags without reading anything, which is handy in tests.

## Usage

```go
package main

import (
	"log"

	"github.com/moeghifar/libgo/pkg/envy"
)

type Config struct {
	Port         int      `env:"APP_PORT" default:"8080"`
	Debug        bool     `env:"DEBUG" default:"false"`
	APIKey       string   `env:"API_KEY" required:"true"`
	AllowedHosts []string `env:"ALLOWED_HOSTS" default:"localhost"` // ALLOWED_HOSTS=a.com,b.com

	Database struct {
		DSN string `env:"DB_DSN" required:"true"`
	}
}

func main() {
	var cfg Config
	if err := envy.Load(&cfg); err != nil {
		log.Fatal(err)
	}
	log.Printf("listening on :%d", cfg.Port)
}
```

## Reference

| Function | What it does |
| --- | --- |
| `Load(&cfg)` | Applies `default` tags, loads `.env` if the file exists, then reads the environment. Returns an error for an empty `required` field or a value that does not parse. |
| `Defaults(&cfg)` | Applies `default` tags only: no `.env`, no environment, and `required` is not checked. |

| Tag | Meaning |
| --- | --- |
| `env:"NAME"` | Variable to read. A field without it is skipped (nested structs are always walked). |
| `default:"value"` | Value used when the variable is unset or empty |
| `required:"true"` | The field must end up non-empty |

## Build tags

`.env` support links godotenv (about 16 KB in a stripped binary, measured with Go 1.27.4). Containers and Kubernetes set the real environment, so production builds can leave it out:

```sh
go build -tags libgo_envy_slim .
```

With the tag, `Load` reads only defaults and the environment.

## Not included

- No variable expansion (`${OTHER}`) and no file formats other than `.env`.
- No validation beyond `required`. Check value ranges in your own code after `Load`.
- No hot reload. Configuration is read once at startup.

## License

Apache 2.0, see [LICENSE](../../LICENSE)
