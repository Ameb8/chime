# chime — Design Document

> Cross-platform CLI notification daemon for coding agents.  
> Alerts you when an agent finishes a task or waits for input, on the same machine or over the LAN.

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [CLI Interface](#cli-interface)
4. [HTTP API](#http-api)
5. [Configuration](#configuration)
6. [Notification Backends](#notification-backends)
7. [Hook Scripts](#hook-scripts)
8. [Project Structure](#project-structure)
9. [Key Design Decisions](#key-design-decisions)
10. [Future Work](#future-work)

---

## Overview

Chime has two roles that can run on different machines:

- **Server** — runs on the user's laptop. Receives notification events over HTTP and dispatches toasts and/or sounds.
- **Client** — a thin CLI shipped in the same binary. Sends a notification event to the server. Used directly by agent hook scripts.

The same `chime` binary covers both roles. On a remote agent machine the user only needs the binary to send events; the server stays on the laptop.

```
[Claude Code hook] ──► chime notify --event complete ──► HTTP POST /notify ──► [chime server on laptop]
                                                                                        │
                                                                               toast + sound
```

---

## Architecture

### Component Overview

```
┌──────────────────────────────────────────────────────┐
│  chime binary                                        │
│                                                      │
│  ┌─────────────┐    ┌────────────────────────────┐   │
│  │  CLI layer  │    │  Server (HTTP)             │   │
│  │  (cobra)    │    │  POST /notify              │   │
│  │             │    │  GET  /health              │   │
│  │  notify     │    │                            │   │
│  │  start      │    │  ┌──────────────────────┐  │   │
│  │  stop       │    │  │  Dispatcher          │  │   │
│  │  status     │    │  │  routes event type   │  │   │
│  │  install    │    │  │  to backend(s)       │  │   │
│  └──────┬──────┘    │  └──────────┬───────────┘  │   │
│         │           │             │              │   │
│         │ HTTP      │    ┌────────┴─────────┐    │   │
│         └───────────┼───►│  Backends        │    │   │
│                     │    │  ToastBackend    │    │   │
│                     │    │  SoundBackend    │    │   │
│                     │    └──────────────────┘    │   │
│                     └────────────────────────────┘   │
│                                                      │
│ internal/config · internal/paths · internal/exitcode │
└──────────────────────────────────────────────────────┘
```

### Request Lifecycle

1. Agent hook script calls `chime notify --event complete` (or `--event waiting`).
2. `chime notify` reads server address and API key from config, sends `POST /notify` with a JSON payload.
3. The server validates the API key from the `Authorization: Bearer <key>` header.
4. The dispatcher looks up which backends are enabled for this event type.
5. Each enabled backend fires concurrently (toast and/or sound).
6. Server responds `200 OK` or an error status; the hook script exits.

### Daemon Model

**Current scope:** `chime start` runs in the foreground (`--foreground` is the only implemented mode). The server blocks until `SIGINT` or `SIGTERM`, then shuts down gracefully and exits 0. Users manage the process with their own tooling (a separate terminal, tmux, etc.).

The PID file path (`~/.local/share/chime/chime.pid`) and background daemonize path are stubbed for future implementation. `chime stop` and `chime status` read the PID file and are implemented; they require the server to have been started with a future background mode or a manually written PID file.

Background daemonizing and `chime service install` (launchd/systemd) are deferred to a future release — see [Future Work](#future-work).

---

## CLI Interface

All commands follow the pattern `chime <command> [flags]`.

### `chime start`

Start the notification server. Currently runs in the foreground only.

```
chime start [flags]

Flags:
  --bind string    Address to listen on (default "0.0.0.0:7777")
  --log string     Log file path (default ~/.local/share/chime/chime.log)
```

On first run, if no API key exists in the config, one is generated and printed before the server starts.

```
$ chime start
API key: chime_a3f9...c821
Add to remote machines: export CHIME_KEY=chime_a3f9...c821
Server listening on 0.0.0.0:7777
^C
Server stopped.
```

### `chime stop`

Stop the background server.

```
chime stop
```

Sends `SIGTERM` to the PID in the PID file, then waits up to 5 seconds for the process to exit.

### `chime status`

Print server state.

```
$ chime status
Server: running (PID 84321)
Listening: 0.0.0.0:7777
Uptime: 3h 14m
Log: ~/.local/share/chime/chime.log
```

```
$ chime status
Server: stopped
```

### `chime notify`

Send a notification event to the server. Used by hook scripts.

```
chime notify [flags]

Flags:
  --event string     Event type: complete | waiting  (required)
  --agent string     Agent name, e.g. claude-code, codex, aider
  --message string   Optional human-readable detail from the agent
  --server string    Server URL (overrides config/env)
  --key string       API key (overrides config/env)
```

Examples:

```sh
# Minimal — from a Claude Code Stop hook
chime notify --event complete --agent claude-code

# With a message — from an agent waiting for permission
chime notify --event waiting --agent codex --message "Needs permission to run rm -rf"
```

For exit code details, see CLI_SPEC.md. Hook scripts should not block the agent on failure — a `|| true` suffix is recommended.

### `chime install`

Print hook script snippets for a given agent tool. Does not write any files.

```
chime install <agent> [flags]

Agents: claude-code | codex | aider

Flags:
  --server string   Server URL to embed in snippets (default from config)
  --key string      API key to embed in snippets (default from config)
```

Example:

```
$ chime install claude-code
# Add the following to your Claude Code hooks config:
# ~/.claude/settings.json → "hooks"

{
  "Stop": [
    {
      "matcher": "",
      "hooks": [
        {
          "type": "command",
          "command": "chime notify --event complete --agent claude-code"
        }
      ]
    }
  ],
  "PreToolUse": [
    {
      "matcher": "Bash",
      "hooks": [
        {
          "type": "command",
          "command": "chime notify --event waiting --agent claude-code --message \"$CLAUDE_TOOL_INPUT\""
        }
      ]
    }
  ]
}
```

### `chime config`

View and edit configuration.

```
chime config show               # Print current config as YAML
chime config set <key> <value>  # Set a config value
chime config key                # Print the current API key
chime config key rotate         # Generate and save a new API key
```

---

## HTTP API

The server exposes two endpoints.

### `POST /notify`

Send a notification event.

**Headers:**
```
Authorization: Bearer <api-key>
Content-Type: application/json
```

**Request body:**
```json
{
  "event":   "complete",
  "agent":   "claude-code",
  "message": "Task finished"
}
```

| Field     | Type   | Required | Description                          |
|-----------|--------|----------|--------------------------------------|
| `event`   | string | yes      | `complete` or `waiting`              |
| `agent`   | string | no       | Name of the agent tool               |
| `message` | string | no       | Human-readable detail from the agent |

**Responses:**

| Status | Meaning                        |
|--------|--------------------------------|
| 200    | Event accepted and dispatched  |
| 400    | Malformed JSON or missing event|
| 401    | Missing or invalid API key     |
| 422    | Unknown event type             |
| 500    | Backend dispatch error         |

**Response body (200):**
```json
{
  "ok": true,
  "event": "complete"
}
```

### `GET /health`

Unauthenticated. Returns server status for `chime status` and monitoring.

**Response (200):**
```json
{
  "ok": true,
  "version": "0.1.0",
  "uptime_seconds": 11640
}
```

---

## Configuration

Config file lives at `~/.config/chime/config.yaml` (XDG-compliant). All values can be overridden by environment variables prefixed `CHIME_` or by flags.

### Full config reference

```yaml
# Server bind address
server:
  bind: "0.0.0.0:7777"

# API key for authenticating notify requests
auth:
  key: "chime_a3f9...c821"   # auto-generated on first start

# Default server URL for the notify client
client:
  server: "http://192.168.1.10:7777"

# Notification backends
notifications:
  # Toast (OS notification popup)
  toast:
    enabled: true
    events: [complete, waiting]   # which events trigger a toast

  # Sound
  sound:
    enabled: true
    events: [complete, waiting]
    # Per-event sound files. Leave empty to use built-in defaults.
    complete_sound: ""
    waiting_sound: ""

# Logging
log:
  level: "info"        # debug | info | warn | error
  file: ""             # default: ~/.local/share/chime/chime.log
```

### Environment variable overrides

| Variable          | Config key            |
|-------------------|-----------------------|
| `CHIME_KEY`       | `auth.key`            |
| `CHIME_SERVER`    | `client.server`       |
| `CHIME_BIND`      | `server.bind`         |
| `CHIME_LOG_LEVEL` | `log.level`           |

`CHIME_KEY` and `CHIME_SERVER` are the two variables hook scripts on remote machines need to set.

---

## Notification Backends

Backends are defined by an interface, making it straightforward to add new ones later.

```go
// internal/notify/backend.go
type Backend interface {
    Name() string
    Supports(event Event) bool
    Fire(n Notification) error
}
```

### ToastBackend

Dispatches OS-level notification popups.

| OS      | Mechanism                        | Dependency         |
|---------|----------------------------------|--------------------|
| macOS   | `osascript -e 'display notification ...'` | none (built-in) |
| Linux   | `notify-send`                    | libnotify-bin      |
| Windows | PowerShell `New-BurntToastNotification` | BurntToast module |

Title format: `chime — <Agent>` (e.g. `chime — claude-code`)  
Body: `Task complete` or `Waiting for input` + message if present.

### SoundBackend

Plays an audio file on notification.

| OS      | Mechanism    | Dependency |
|---------|--------------|------------|
| macOS   | `afplay`     | none       |
| Linux   | `paplay`     | pulseaudio |
| Windows | PowerShell `[System.Media.SoundPlayer]` | none |

Built-in default sounds are embedded in the binary via `go:embed` so no external files are needed out of the box. Users can override with their own paths in config.

### Dispatcher

```go
// internal/notify/dispatcher.go
type Dispatcher struct {
    backends []Backend
}

func (d *Dispatcher) Dispatch(n Notification) error {
    var wg sync.WaitGroup
    for _, b := range d.backends {
        if b.Supports(n.Event) {
            wg.Add(1)
            go func(b Backend) {
                defer wg.Done()
                b.Fire(n)
            }(b)
        }
    }
    wg.Wait()
    return nil
}
```

Backends fire concurrently. Individual backend errors are logged but do not fail the HTTP response — a broken sound backend should never suppress the toast.

---

## Hook Scripts

Hook scripts live in `hooks/` in the repo. Users copy the relevant snippet into their agent config.

### `hooks/claude-code/`

Claude Code supports hooks via `~/.claude/settings.json`. Two hook points are relevant:

- **`Stop`** — fires when the agent finishes a task → `complete` event
- **`PreToolUse` with `Bash` matcher** — fires before running a bash command requiring confirmation → `waiting` event

See `hooks/claude-code/README.md` for the exact JSON to paste.

### `hooks/codex/`

Codex CLI supports `~/.codex/config.yaml` hooks:

```yaml
hooks:
  after_task: "chime notify --event complete --agent codex"
  before_approval: "chime notify --event waiting --agent codex"
```

See `hooks/codex/README.md`.

### `hooks/aider/`

Aider supports `--after-reply` and `--no-auto-commits` hooks. The simplest approach is a wrapper script:

```sh
#!/usr/bin/env sh
# hooks/aider/complete.sh
chime notify --event complete --agent aider
```

Add to aider invocation: `aider --after-reply hooks/aider/complete.sh`

See `hooks/aider/README.md`.

---

## Project Structure

```
chime/
├── cmd/
│   └── chime/
│       └── main.go                  # Entry point; calls cli.NewRootCmd().Execute()
│
├── internal/
│   ├── cli/                         # Cobra command implementations
│   │   ├── root.go                  # Root command, NewRootCmd(), registers all subcommands
│   │   ├── start.go                 # chime start
│   │   ├── stop.go                  # chime stop
│   │   ├── status.go                # chime status
│   │   ├── notify.go                # chime notify
│   │   ├── install.go               # chime install
│   │   └── config.go                # chime config
│   │
│   ├── server/
│   │   ├── server.go                # HTTP server setup, graceful shutdown
│   │   ├── handler.go               # POST /notify, GET /health handlers
│   │   └── middleware.go            # Auth middleware (API key check)
│   │
│   ├── notify/
│   │   ├── backend.go               # Backend interface + Notification type
│   │   ├── dispatcher.go            # Concurrent backend dispatch
│   │   ├── toast_darwin.go          # macOS toast via osascript
│   │   ├── toast_linux.go           # Linux toast via notify-send
│   │   ├── toast_windows.go         # Windows toast via PowerShell
│   │   ├── sound_darwin.go          # macOS sound via afplay
│   │   ├── sound_linux.go           # Linux sound via paplay
│   │   └── sound_windows.go         # Windows sound via PowerShell
│   │
│   ├── config/
│   │   ├── config.go                # Config struct, defaults, viper binding
│   │   └── key.go                   # API key generation and storage
│   │
│   ├── exitcode/
│   │   ├── codes.go                 # Exit code constants (0–5)
│   │   └── exitcode.go              # Error sentinel type checked by main.go
│   │
│   ├── paths/
│   │   └── paths.go                 # Canonical data/config directory paths
│   │
│   ├── daemon/
│   │   ├── daemon.go                # Fork-and-daemonize logic
│   │   └── pid.go                   # PID file read/write/check
│   │
│   └── client/
│       └── client.go                # HTTP client for chime notify
│
├── assets/
│   ├── sounds/
│   │   ├── complete.aiff            # Default complete sound (embedded)
│   │   └── waiting.aiff             # Default waiting sound (embedded)
│   └── embed.go                     # go:embed declarations
│
├── hooks/
│   ├── claude-code/
│   │   ├── README.md
│   │   └── settings.json.snippet
│   ├── codex/
│   │   ├── README.md
│   │   └── config.yaml.snippet
│   └── aider/
│       ├── README.md
│       ├── complete.sh
│       └── waiting.sh
│
├── docs/
│   ├── DESIGN.md                    # This document
│   ├── quickstart.md
│   └── remote-setup.md
│
├── .github/
│   └── workflows/
│       └── release.yml              # GoReleaser CI
│
├── .goreleaser.yaml                 # Cross-platform build config
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### Package responsibilities

| Package | Responsibility |
|---|---|
| `cmd/chime` | Binary entry point only; no logic |
| `internal/cli` | Command parsing, flag wiring, calls into other packages |
| `internal/server` | HTTP lifecycle, request validation, auth |
| `internal/notify` | Backend interface, OS-specific implementations, dispatcher |
| `internal/config` | Viper setup, typed Config struct, key generation |
| `internal/exitcode` | Exit code constants and sentinel error type checked by main.go |
| `internal/paths` | Canonical config file, data directory, PID file, and log file paths |
| `internal/daemon` | PID file read/write/check; daemonize logic (future) |
| `internal/client` | HTTP client used by `chime notify` |
| `assets` | Embedded sound files |

---

## Key Design Decisions

### Event type extensibility

Events are a typed string constant, not an enum, so adding a new event type requires no schema migration — just a new constant and a backend `Supports()` update:

```go
// internal/notify/backend.go
type Event string

const (
    EventComplete Event = "complete"
    EventWaiting  Event = "waiting"
    // EventError  Event = "error"  // future
)
```

The HTTP API accepts any string for `event` and returns `422` for unknown types, so old clients fail gracefully against a newer server that has added event types.

### OS-specific backends via build tags

Sound and toast backends use Go build tags rather than runtime `GOOS` switches. This keeps each file simple and avoids shipping dead code:

```go
//go:build darwin
// +build darwin

package notify

// ToastBackend implementation for macOS
```

### API key auth

The API key is a randomly generated 32-byte hex string prefixed `chime_`. It is stored in the config file (mode `0600`) and never logged. The server rejects requests without a matching `Authorization: Bearer` header with a `401`. No expiry for MVP — `chime config key rotate` replaces it.

### No dependency on external notification binaries at build time

All OS-tool invocations (`osascript`, `notify-send`, `afplay`) are runtime `exec.Command` calls. The binary builds cleanly on any platform; a missing OS tool produces a logged error and a graceful fallback (e.g. toast fails silently, sound still plays).

---

## Future Work

Roughly in priority order:

- `chime start` background mode — fork/daemonize, PID file management, `chime stop` and `chime status` fully functional.
- `chime service install` — generates and loads a launchd plist (macOS) or systemd unit (Linux) for auto-start on login.
- `chime install <agent>` with write mode — actually patches agent config files rather than printing snippets.
- Additional event types — `error`, `heartbeat`, `started`.
- Web UI — a minimal local status page at `http://localhost:7777` showing recent notification history.
- Multiple server targets — allow `chime notify` to fan out to multiple server addresses (e.g. laptop + desktop).
- Per-agent sound profiles — different sounds for different tools.
- Cross-platform packaging — Homebrew formula, `.deb`/`.rpm`, Winget manifest via GoReleaser.