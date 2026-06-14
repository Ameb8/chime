# `internal/config` — Package Spec

> Authoritative specification for the `internal/config` package in chime.  
> All config loading, key generation, and flag/env resolution logic lives here.

---

## Responsibilities

- Define the canonical `Config` struct and all sub-structs.
- Load config from the XDG config file (`~/.config/chime/config.yaml`) via Viper, creating the file with defaults if it does not exist.
- Expose a `Resolve` helper for 3-tier flag/env/config-file precedence.
- Generate and persist API keys (`internal/config/key.go`).
- Export a `Save` function that writes the in-memory config back to the config file.

The package is side-effect-free with respect to API keys: `Load` never generates a key. Key generation is the caller's responsibility (`chime start`).

---

## Files

```
internal/config/
├── config.go   # Config struct, Load(), Save(), defaults
└── key.go      # GenerateKey(), key format constants
```

`Resolve` lives in `config.go` because it is tightly coupled to how config values are loaded and referenced, and it has no external dependencies that would warrant its own file.

---

## `config.go`

### Config struct

```go
type Config struct {
    Server        ServerConfig        `mapstructure:"server"`
    Auth          AuthConfig          `mapstructure:"auth"`
    Client        ClientConfig        `mapstructure:"client"`
    Notifications NotificationsConfig `mapstructure:"notifications"`
    Log           LogConfig           `mapstructure:"log"`
}

type ServerConfig struct {
    Bind string `mapstructure:"bind"` // e.g. "0.0.0.0:7777"
}

type AuthConfig struct {
    Key string `mapstructure:"key"` // set by chime start on first run
}

type ClientConfig struct {
    Server string `mapstructure:"server"` // e.g. "http://192.168.1.10:7777"
}

type NotificationsConfig struct {
    Toast ToastConfig `mapstructure:"toast"`
    Sound SoundConfig `mapstructure:"sound"`
}

type ToastConfig struct {
    Enabled bool     `mapstructure:"enabled"`
    Events  []string `mapstructure:"events"` // e.g. ["complete", "waiting"]
}

type SoundConfig struct {
    Enabled       bool     `mapstructure:"enabled"`
    Events        []string `mapstructure:"events"`
    CompleteSound string   `mapstructure:"complete_sound"` // empty = use embedded default
    WaitingSound  string   `mapstructure:"waiting_sound"`
}

type LogConfig struct {
    Level string `mapstructure:"level"` // "debug" | "info" | "warn" | "error"
    File  string `mapstructure:"file"`  // empty = use paths.LogFile()
}
```

All fields use `mapstructure` tags to match Viper's unmarshalling. `yaml` tags are not needed separately — Viper reads YAML and maps via `mapstructure`.

### Defaults

Defaults are registered with Viper before loading. They must exactly match the YAML keys:

| Viper key                          | Default value               |
|------------------------------------|-----------------------------|
| `server.bind`                      | `"0.0.0.0:7777"`           |
| `auth.key`                         | `""`                        |
| `client.server`                    | `""`                        |
| `notifications.toast.enabled`      | `true`                      |
| `notifications.toast.events`       | `["complete", "waiting"]`   |
| `notifications.sound.enabled`      | `true`                      |
| `notifications.sound.events`       | `["complete", "waiting"]`   |
| `notifications.sound.complete_sound` | `""`                      |
| `notifications.sound.waiting_sound`  | `""`                      |
| `log.level`                        | `"info"`                    |
| `log.file`                         | `""`                        |

### Environment variable bindings

Viper `BindEnv` calls map environment variables to config keys. Only the two variables that hook scripts on remote machines need are bound:

| Env var        | Viper key       |
|----------------|-----------------|
| `CHIME_KEY`    | `auth.key`      |
| `CHIME_SERVER` | `client.server` |
| `CHIME_BIND`   | `server.bind`   |
| `CHIME_LOG_LEVEL` | `log.level`  |

These are registered once in `Load`, not scattered across command files.

### `Load() (*Config, error)`

```go
func Load() (*Config, error)
```

**Behavior:**

1. Determine the config file path via `paths.ConfigFile()`.
2. Register all defaults with Viper.
3. Register all `BindEnv` mappings.
4. Set the config file path on the Viper instance.
5. Call `viper.ReadInConfig()`.
   - If the file does not exist, detect it with the following pattern and create the parent directory with `os.MkdirAll` (mode `0700`), then call `Save` with the default config to write the file before continuing:
     ```go
     var notFound viper.ConfigFileNotFoundError
     if errors.As(err, &notFound) || os.IsNotExist(err) {
         // first run — write defaults to disk
     } else {
         return nil, err
     }
     ```
     Do not use `errors.Is` here — `viper.ConfigFileNotFoundError` is a struct type, not a sentinel value, so only `errors.As` works. The `os.IsNotExist` check covers the case where Viper locates the file path but the OS reports it missing before Viper can wrap the error.
   - Any other read error: return it.
6. Unmarshal into a `Config` struct via `viper.Unmarshal(&cfg)`.
7. Return `&cfg, nil`.

`Load` is called once by the root command during `PersistentPreRunE`. The resulting `*Config` is stored on the root command struct and passed to all subcommands by pointer.

**Viper instance:** Use the global Viper instance (`viper.SetConfigFile`, `viper.ReadInConfig`, etc.) — not a new `viper.New()`. A package-level `var v = viper.New()` approach is acceptable if you want test isolation, but document the choice. The global instance is fine for MVP.

### `Save(cfg *Config) error`

```go
func Save(cfg *Config) error
```

Marshals `cfg` to YAML and writes it to the path returned by `paths.ConfigFile()`, creating parent directories as needed. File permissions must be `0600` (owner read/write only) because the file contains the API key.

This is used by:
- `Load`, when creating the config file for the first time.
- `chime config set` and `chime config key rotate`, after mutating the in-memory config.

Implementation note: Viper's `WriteConfigAs` can be used here, but it requires the Viper instance to already have the values set. An alternative is to marshal the struct directly with `gopkg.in/yaml.v3` and write with `os.WriteFile`. Either approach is fine; pick one and be consistent.

### `Resolve(flagVal, envVar, configVal string) string`

```go
func Resolve(flagVal, envVar, configVal string) string
```

Returns the first non-empty value in priority order:

1. `flagVal` — value passed via CLI flag (empty string if flag was not set)
2. `os.Getenv(envVar)` — environment variable looked up by name
3. `configVal` — value from the loaded `Config` struct

```go
func Resolve(flagVal, envVar, configVal string) string {
    if flagVal != "" {
        return flagVal
    }
    if v := os.Getenv(envVar); v != "" {
        return v
    }
    return configVal
}
```

**Usage in commands:**

```go
// internal/cli/notify.go
serverURL := config.Resolve(serverFlag, "CHIME_SERVER", cfg.Client.Server)
apiKey    := config.Resolve(keyFlag,    "CHIME_KEY",    cfg.Auth.Key)
```

Note: Viper already handles env var overrides for values read via `viper.Unmarshal`. `Resolve` is specifically for the case where a command flag can override what Viper already resolved — i.e. a third layer on top of Viper's own two layers (env + file). Commands that do not have overridable flags do not need `Resolve`.

---

## `key.go`

### Constants

```go
const (
    KeyPrefix = "chime_"
    keyBytes  = 16 // 16 random bytes → 32 hex chars
)
```

The full key format is `chime_` + 32 lowercase hex characters, e.g. `chime_a3f9b2c1d4e5f6a7b8c9d0e1f2a3b4c5`.

### `GenerateKey() (string, error)`

```go
func GenerateKey() (string, error)
```

Generates a new API key using `crypto/rand`.

```go
func GenerateKey() (string, error) {
    b := make([]byte, keyBytes)
    if _, err := rand.Read(b); err != nil {
        return "", fmt.Errorf("generate api key: %w", err)
    }
    return KeyPrefix + hex.EncodeToString(b), nil
}
```

This function only generates the key. Persisting it (setting `cfg.Auth.Key` and calling `Save`) is the caller's responsibility (`chime start`).

---

## How the root command wires this up

To be clear about the seam between `internal/config` and `internal/cli`:

```go
// internal/cli/root.go

type RootCmd struct {
    cmd *cobra.Command
    cfg *config.Config
}

func NewRootCmd() *cobra.Command {
    r := &RootCmd{}

    r.cmd = &cobra.Command{
        Use:   "chime",
        Short: "Cross-platform CLI notification daemon for coding agents",
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            // Skip config loading for commands that don't need it.
            // Cobra runs PersistentPreRunE for every command including
            // "help" and "completion", which should work without a config file.
            switch cmd.Name() {
            case "help", "completion":
                return nil
            }
            cfg, err := config.Load()
            if err != nil {
                return fmt.Errorf("load config: %w", err)
            }
            r.cfg = cfg
            return nil
        },
    }

    r.cmd.AddCommand(newStartCmd(&r.cfg))
    r.cmd.AddCommand(newNotifyCmd(&r.cfg))
    // etc.

    return r.cmd
}
```

Subcommands receive `**config.Config` (a pointer to the pointer) so they always see the value that `PersistentPreRunE` sets, regardless of initialization order.

Alternatively, pass `*config.Config` and have `PersistentPreRunE` mutate fields on a pre-allocated struct. Either pattern is fine; the key constraint is that `config.Load()` is called exactly once.

---

## What this package does NOT do

- **No flag parsing.** Cobra owns flags. This package exposes `Resolve` for flag-value integration but does not read `os.Args`.
- **No API key generation on load.** `Load` never calls `GenerateKey`. If `cfg.Auth.Key` is empty after load, that is a valid state — `chime start` handles it.
- **No validation.** Out of scope for now. A `Validate() error` method on `Config` may be added later.
- **No logging configuration.** `Load` returns the log level and path as plain strings. `main.go` uses those values to configure `slog` — the config package itself does not touch `slog`.
- **No global state exposed to callers.** The Viper instance is not exported. Callers interact only with `*Config`, `Load`, `Save`, `Resolve`, and `GenerateKey`.

---

## Dependencies

| Import | Purpose |
|---|---|
| `github.com/spf13/viper` | YAML loading, env var binding, defaults |
| `crypto/rand` | Cryptographically secure key generation |
| `encoding/hex` | Key encoding |
| `os` | Directory creation, file writing, env var lookup |
| `fmt` | Error wrapping |
| `errors` | `errors.As` for config-not-found detection |
| `internal/paths` | Canonical config file and data directory paths |

No other internal packages. `internal/config` must not import `internal/cli`, `internal/server`, or `internal/notify` — it sits at the bottom of the dependency graph.