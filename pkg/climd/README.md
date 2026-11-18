# climd

Command-line apps described by one struct: a small alternative to Cobra.

The whole CLI (commands, subcommands, flags and handlers) is one `AppConfig` literal. There is no code generation, no global registry and no `init()` wiring. Standard library only.

```sh
go get github.com/moeghifar/libgo/pkg/climd
```

## Features

- **One struct:** `AppConfig{Commands: []Command{...}}` describes the entire app.
- **Commands and subcommands:** `app serve`, `app db migrate`.
- **Flags:** long (`--output dir`) and short (`-o dir`), with values or as booleans (`--verbose`).
- **Required flags:** a missing `Required` flag is an error naming it.
- **Default command:** `AppConfig{Default: "serve"}` runs `serve` when no arguments are given.
- **Built-in help and version:** `--help`/`-h` and `--version`/`-v`, generated from the struct.
- **Testable:** `Run(cfg, args)` returns the error; `Execute(cfg)` is the `main` wrapper that prints it and exits 1.

## Usage

```go
package main

import (
	"context"
	"fmt"

	"github.com/moeghifar/libgo/pkg/climd"
)

func main() {
	climd.Execute(climd.AppConfig{
		Name:        "myapp",
		Version:     "1.0.0",
		Description: "Example service",
		Default:     "serve",
		Commands: []climd.Command{
			{
				Name:  "serve",
				Short: "Start the HTTP server",
				Flags: []climd.Flag{{Name: "addr", Short: "a", Usage: "listen address"}},
				Run: func(ctx context.Context, args []string, flags map[string]string) error {
					addr := flags["addr"]
					if addr == "" {
						addr = ":8080"
					}
					fmt.Println("listening on", addr)
					return nil
				},
			},
			{
				Name:  "db",
				Short: "Database tasks",
				SubCommands: []climd.SubCommand{
					{
						Name:  "migrate",
						Short: "Apply migrations",
						Flags: []climd.Flag{{Name: "dir", Usage: "migrations directory", Required: true}},
						Run: func(ctx context.Context, args []string, flags map[string]string) error {
							fmt.Println("migrating from", flags["dir"])
							return nil
						},
					},
				},
			},
		},
	})
}
```

```sh
myapp                          # runs the default command, serve
myapp serve --addr :9000
myapp db migrate --dir ./sql
myapp --help
myapp --version
```

## Reference

| Type | Fields |
| --- | --- |
| `AppConfig` | `Name`, `Version`, `Description`, `Commands`, `Default` |
| `Command` | `Name`, `Short`, `Long`, `Flags`, `SubCommands`, `Run` |
| `SubCommand` | `Name`, `Short`, `Long`, `Flags`, `Run` |
| `Flag` | `Name`, `Short`, `Usage`, `Required` |

`Run` receives the context, the positional arguments, and the flags as `map[string]string`.

Flag parsing rules:

- A flag takes the next argument as its value unless that argument starts with `-`. Otherwise the flag is a boolean and maps to `""`, so check presence with `_, ok := flags["verbose"]`.
- Values are separated by a space. `--addr=:9000` is not parsed as a flag.
- The map key is the form the user typed: `-a` is stored under `"a"`, `--addr` under `"addr"`. If you declare a `Short` form, read both keys.
- Unknown flags are passed through as positional arguments.
- A command with subcommands but no subcommand given prints that command's help.

## Not included

- No `--name=value` syntax, no typed flag values, and no flag defaults. Values are strings; parse them in `Run`.
- No shell completion and no generated man pages.
- No signal handling. `Run` gets `context.Background()`; wrap it with `signal.NotifyContext` inside your command.

## License

Apache 2.0, see [LICENSE](../../LICENSE)
