# AGENTS.md

Chime is a cross-platform CLI notification daemon for coding agents. The same binary acts as both server (receives HTTP notifications, fires toasts/sounds) and client (`chime notify` sends events to the server).

Before writing any code, read:
- `docs/DESIGN.md` — authoritative source for architecture, package responsibilities, and project structure.
- `docs/CLI_SPEC.md` — authoritative source for command behavior, flags, and exit codes. When uncertain about what a command should do, check here first.

---

## Project Structure

All logic lives in `internal/`. `cmd/chime/main.go` is the entry point only — no logic belongs there. Refer to DESIGN.md for the full package breakdown.

---

## Conventions

### Cobra command wiring

All subcommands are registered in `root.go`'s `init()` via `rootCmd.AddCommand(...)`. Do not use per-file `init()` self-registration.

### Error handling

Commands use `RunE` and return errors up to the root. `main.go` is the only place that writes to stderr and calls `os.Exit`. Do not call `os.Exit`, `log.Fatal`, or write to `os.Stderr` directly inside any command file.

```go
// main.go — only place that exits
func main() {
    if err := cli.NewRootCmd().Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}

// command files — return errors, never exit
RunE: func(cmd *cobra.Command, args []string) error {
    return doThing()
}
```

For commands that need a specific exit code (not 1), use a sentinel error type defined in `internal/exitcode/`:

```go
// internal/exitcode/exitcode.go
type Error struct {
    Code    int
    Message string
}
func (e *Error) Error() string { return e.Message }
```

`main.go` checks for `*exitcode.Error` before falling back to exit 1. Exit code constants live in `internal/exitcode/codes.go`. The full exit code table is in CLI_SPEC.md.

### Config

Config is a typed struct, not raw viper calls. Viper is used only in `internal/config/` to load and unmarshal into the struct. Commands receive config as a value — they do not call viper directly.

```go
// internal/config/config.go
type Config struct {
    Server        ServerConfig
    Auth          AuthConfig
    Client        ClientConfig
    Notifications NotificationsConfig
    Log           LogConfig
}
```

The root command loads config once and passes it to subcommands via a shared pointer or cobra's context. Do not call `viper.GetString(...)` outside of `internal/config/`.

### Flag and env var precedence

For `--server` and `--key` (used by `notify` and `install`), resolution order is:

1. CLI flag
2. `CHIME_SERVER` / `CHIME_KEY` environment variable
3. `client.server` / `auth.key` in config file

Implement this resolution in `internal/config/` as a helper, not inline in each command.

### File paths

Use the `internal/paths/` package for all canonical locations (config file, PID file, log file, data directory). Do not hardcode or inline path construction elsewhere.

### OS-specific notification backends

Toast and sound backends use Go build tags — one file per platform. Do not use `runtime.GOOS` switches.

```go
//go:build darwin

package notify
// macOS-specific implementation
```

Files: `toast_darwin.go`, `toast_linux.go`, `toast_windows.go`, `sound_darwin.go`, etc.

### Embedded assets

Sound files are embedded via `go:embed` in `assets/embed.go` and exported as package-level variables. Reference them from there — do not re-embed in other packages.

---

## Output and Logging

Use `log/slog` with the default logger. In `main.go`, configure the default logger at startup based on the resolved log level from config. Everywhere else, call `slog.Info(...)`, `slog.Debug(...)`, etc. directly — do not import or configure slog outside of `main.go`.

**User-facing CLI output** (command results, keys, snippets, status): use `fmt.Fprintln(os.Stdout, ...)`. Errors returned to the user go to stderr via the error-bubbling pattern — do not write to stderr directly in command files.

**Server operational logs** (startup, shutdown, request traces, backend errors): use `log/slog`. These go to the log file only and are never shown in the terminal during normal operation. In `main.go`, configure the default logger at startup based on the resolved log level from config. Everywhere else, call `slog.Info(...)`, `slog.Debug(...)`, etc. directly — do not import or configure slog outside of `main.go`.

Do not use `slog` for CLI output, and do not use `fmt` for server logs. If you are unsure which applies, ask: is this a message for the user running a command, or a record for the developer diagnosing a server issue?

---

## Code Quality

Before finishing any task, run:

```sh
gofmt -w .
golangci-lint run        # fix any issues before marking task complete
go test ./...
```

A `.golangci.yaml` config lives at the repo root — do not disable linters inline with `//nolint` without a comment explaining why.

---

## Out of Scope (for now)

- `chime start` daemonizing — implement `--foreground` mode only; stub or `TODO` the fork/background path.
- `chime service install` (launchd/systemd integration)
- `chime install` writing to agent config files — print snippets only