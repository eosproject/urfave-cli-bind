# urfave-cli-bind

[![Go Reference](https://pkg.go.dev/badge/github.com/eosproject/urfave-cli-bind.svg)](https://pkg.go.dev/github.com/eosproject/urfave-cli-bind)

A tiny helper that maps `urfave/cli/v3` flags to and from Go structs. Define your configuration once with struct tags, generate the corresponding `cli.Flag`s, and bind parsed values back into the struct inside your command action.

This repository is an add-on for [`github.com/urfave/cli/v3`](https://github.com/urfave/cli/). It layers reflection helpers on top of the original CLI runtime and is neither a fork nor a replacement for `urfave/cli` itself.

## Features
- Reflect-based flag generation via `FlagsFromStruct` for primitives, durations, times, UUIDs, and slices
- Tag-driven defaults (`cliDefault`), usage strings (`cliUsage`), prefixes for nested structs (`cliPrefix`), and `omitempty`
- Works with concrete structs or pointers, including anonymous/embedded structs for flattening
- Binds directly from `*cli.Command` using the same metadata so there is no duplicate wiring

## Installation
```bash
go get github.com/eosproject/urfave-cli-bind
```

## Quick start
```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/urfave/cli/v3"
    clibind "github.com/eosproject/urfave-cli-bind"
)

type Config struct {
    Name  string        `cli:"name,n"  cliDefault:"guest" cliUsage:"User name"`
    Count int           `cli:"count"   cliDefault:"3"     cliUsage:"How many"`
    Delay time.Duration `cli:"delay"   cliDefault:"250ms" cliUsage:"Wait duration"`
}

func main() {
    app := &cli.App{
        Flags: clibind.FlagsFromStruct(Config{}),
        Action: func(ctx context.Context, cmd *cli.Command) error {
            var cfg Config
            if err := clibind.Bind(cmd, &cfg); err != nil {
                return err
            }
            log.Printf("config: %+v", cfg)
            return nil
        },
    }
    if err := app.Run(context.Background(), []string{"demo"}); err != nil {
        log.Fatal(err)
    }
}
```

## Even quickier start
```go
import (
    "context"
    "fmt"
    "os"

    "github.com/urfave/cli/v3"
    clibind "github.com/eosproject/urfave-cli-bind"
)

type ServerCfg struct {
    Addr string `cli:"addr"   cliDefault:"127.0.0.1:8080"`
    TLS  bool   `cli:"tls"`
}

func main() {
    app := clibind.CommandWithBinding(nil, "app", 
        func(ctx context.Context, cfg ServerCfg) error {
            fmt.Printf("serving %s tls=%v\n", cfg.Addr, cfg.TLS)
            return nil
        },
    )

    _ = app.Run(context.Background(), os.Args)
}
```

## Tag reference
| Tag | Purpose |
| --- | --- |
| `cli:"name,alias,alias2"` | Primary flag name plus optional aliases; add `,omitempty` to skip unset optional flags. |
| `cliDefault:"value"` | Default value shown in help and used when the flag is missing. |
| `cliUsage:"text"` | Usage/help text surfaced in `urfave/cli` output. |
| `cliPrefix:"foo."` | Applied to every nested field when recursing into a struct field. |
| `cliTimeLayout:"2006-01-02"` | Overrides the RFC3339 default for `time.Time` parsing. |

## Nested structs and prefixes
- Anonymous embedded structs without `cliPrefix` are flattened so their fields become top-level flags.
- Named struct fields can opt-in to namespacing by providing `cliPrefix`. The prefix is prepended to all generated flag names and multi-character aliases, mirroring how `Bind` searches for values.

## Binding rules
- `Bind` requires a non-nil pointer to a struct and mirrors the type handling used in flag generation.
- Required flags are inferred: if a field omits `omitempty` and lacks `cliDefault`, the generated flag is marked as required.
- Slices use comma-separated defaults (`cliDefault:"a,b,c"`), duration fields expect the Go duration syntax, and UUID fields are treated as strings and parsed inside `Bind`.
