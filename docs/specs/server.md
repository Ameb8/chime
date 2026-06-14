# chime server — Implementation Specification

> Behavioral and package-level spec for implementing `chime start` and the HTTP
> server it runs.
>
> Read alongside:
>
> - `docs/design.md` — architecture and current project scope.
> - `docs/specs/cli.md` — command behavior and exit codes. Some daemon-mode
>   conditions in that file are future work; this server spec defines the
>   foreground-only MVP target.
> - `docs/specs/config.md` — config loading, saving, key generation, and
>   flag/env/config precedence.
> - `docs/specs/notify.md` — `internal/notify` public API and dispatcher
>   behavior.

---

## Status And Scope

The current repository has CLI scaffolding, config, paths, exit code handling,
and `internal/notify`. It does **not** yet have `internal/server`, concrete
toast/sound backends, embedded assets, `internal/client`, or `internal/daemon`.

This document is an implementation target for adding the server behavior. It
should not describe missing packages as already present.

### In Scope

- Implement foreground HTTP server behavior for `chime start`.
- Add `internal/server` with HTTP lifecycle, handlers, middleware, and tests.
- Wire `internal/cli/start.go` to load or generate the API key, construct the
  dispatcher, create the server, and run it in the foreground.
- Use the existing `internal/config`, `internal/paths`, `internal/exitcode`, and
  `internal/notify` contracts.
- Keep backend construction compatible with the `notify.Backend` interface,
  even if concrete toast/sound backends are added in separate work.

### Out Of Scope For This Spec

- Background daemonizing/forking.
- `internal/daemon` PID file management.
- `chime service install`.
- `chime install` writing to agent config files.
- Full `chime notify` HTTP client behavior, except where endpoint behavior is
  relevant for server tests.

Until background mode exists, `chime start` and `chime start --foreground`
should run the foreground server. Do not implement fork/daemon behavior as part
of this server work.

---

## Startup Sequence

The root command loads config once in `internal/cli/root.go` and passes it to
subcommands as `**config.Config`. `chime start` must use that loaded config
value rather than calling Viper directly.

When `chime start` runs in the foreground, the following steps happen in order:

1. Read the loaded `*config.Config`.
2. Resolve bind address with `config.Resolve(bindFlag, "CHIME_BIND", cfg.Server.Bind)`.
3. Resolve log file path with: `--log` flag, then `cfg.Log.File`, then `paths.LogFile()`.
4. If `cfg.Auth.Key` is empty, generate a key with `config.GenerateKey()`, assign
   it to `cfg.Auth.Key`, and persist the updated config with `config.Save(cfg)`.
5. If a key was generated in step 4, print the key and setup hint to stdout.
6. Configure `slog` for server/runtime logs.
7. Build the backend list from config. If no concrete backends are available
   yet, pass an empty slice rather than blocking server implementation.
8. Build `notify.NewDispatcher(backends)`.
9. Build `server.New(cfg, dispatcher, options)` or equivalent.
10. Start listening on the TCP socket. Binding failures return immediately.
11. Print `Server listening on <bind>` to stdout after the socket is bound.
12. Block until `SIGINT` or `SIGTERM`.
13. Run graceful shutdown.
14. Print `Server stopped.` to stdout.
15. Return nil so `main.go` exits 0.

Any startup failure returns an error. `main.go` is responsible for writing the
error to stderr and exiting with the appropriate code.

### API Key Display

The API key is printed only when `chime start` generates a new key because none
exists in config.

Example first start:

```text
API key: chime_a3f9b2c1d4e5f6a7b8c9d0e1f2a3b4c5
Add to remote machines: export CHIME_KEY=chime_a3f9b2c1d4e5f6a7b8c9d0e1f2a3b4c5
Server listening on 0.0.0.0:7777
```

On subsequent starts, do not print the existing key. Users can retrieve it with
`chime config key`.

Keys must use the format defined in `docs/specs/config.md`: `chime_` followed
by 32 lowercase hexadecimal characters.

### Startup Testable Conditions

- **SERVER-START-1** When `cfg.Auth.Key` is empty, `chime start` generates a key
  matching `^chime_[0-9a-f]{32}$`.
- **SERVER-START-2** When a key is generated, the updated config file is saved
  with mode `0600`.
- **SERVER-START-3** When a key is generated, stdout includes `API key: <key>`
  before `Server listening on <bind>`.
- **SERVER-START-4** When `cfg.Auth.Key` is already set, stdout does not include
  the API key.
- **SERVER-START-5** `--bind` takes precedence over `CHIME_BIND`, which takes
  precedence over `cfg.Server.Bind`.
- **SERVER-START-6** A port already in use causes `chime start` to return an
  error and exit 1.
- **SERVER-START-7** `chime start --foreground` blocks until `SIGINT` or
  `SIGTERM`.
- **SERVER-START-8** `chime start` without `--foreground` also runs foreground
  mode for the MVP and does not fork.
- **SERVER-START-9** After receiving `SIGTERM`, the foreground process prints
  `Server stopped.` and exits 0.

---

## HTTP Server

The server package should live in `internal/server`.

Suggested public API:

```go
type Server struct {
    // fields unexported
}

type Options struct {
    Bind     string
    LogPath  string
    Version  string
}

func New(cfg *config.Config, dispatcher *notify.Dispatcher, opts Options) *Server
func (s *Server) Start(ctx context.Context) error
func (s *Server) Shutdown(ctx context.Context) error
```

`Start(ctx)` should bind the listener, serve HTTP, and return when the server is
shut down or fails to start. It should not call `os.Exit`, write to stderr, or
configure global CLI behavior.

Signal handling may live in `internal/cli/start.go` or `internal/server`, but it
must be implemented in one place only. Prefer `start.go` owning OS signals and
passing a cancelable context into `Server.Start(ctx)` so the server package is
easy to test without sending real process signals.

### HTTP Properties

| Property | Value |
|---|---|
| Default bind | `0.0.0.0:7777` |
| Protocol | HTTP/1.1, no TLS |
| Read timeout | 5 seconds |
| Write timeout | 10 seconds |
| Idle timeout | 60 seconds |
| Max request body | 1 MiB |
| Shutdown timeout | 5 seconds |

Timeouts belong on the `http.Server` struct. Use the standard library only.

### HTTP Server Testable Conditions

- **SERVER-HTTP-1** `server.New` registers `POST /notify` and `GET /health`.
- **SERVER-HTTP-2** Unknown paths return 404.
- **SERVER-HTTP-3** Unsupported methods on known paths return 405 when practical
  with the chosen mux; otherwise they must not invoke the handler.
- **SERVER-HTTP-4** `http.Server` has read, write, and idle timeouts matching
  this spec.
- **SERVER-HTTP-5** A request body larger than 1 MiB is rejected and does not
  dispatch a notification.
- **SERVER-HTTP-6** `Server.Shutdown(ctx)` calls the underlying HTTP shutdown and
  stops accepting new requests.

---

## Endpoints

### `POST /notify`

Receives one notification event and dispatches it to compatible backends.

#### Request

```http
POST /notify HTTP/1.1
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "event": "complete",
  "agent": "claude-code",
  "message": "Task finished"
}
```

| Field | Type | Required | Constraints |
|---|---|---|---|
| `event` | string | yes | Must be `complete` or `waiting`; unknown values return 422. |
| `agent` | string | no | Trim whitespace; if longer than 64 chars, truncate to 64. |
| `message` | string | no | Trim whitespace; if longer than 512 chars, truncate to 512. |

Malformed JSON returns 400. Missing or blank `event` returns 400. A JSON body
larger than 1 MiB returns 400.

#### Responses

| Status | Condition |
|---|---|
| `200 OK` | Event accepted and dispatch attempted. Backend errors do not affect status. |
| `400 Bad Request` | Malformed JSON, missing event, blank event, or body too large. |
| `401 Unauthorized` | Missing, malformed, or incorrect API key. |
| `422 Unprocessable Entity` | Event is syntactically present but not known. |
| `500 Internal Server Error` | Unexpected panic or server failure. |

Successful response:

```json
{
  "ok": true,
  "event": "complete"
}
```

Error response:

```json
{
  "ok": false,
  "error": "missing event field"
}
```

`error` is a short human-readable string, not a stable machine-readable code.

#### Dispatch Behavior

The handler converts the payload to `notify.Notification` and calls:

```go
err := dispatcher.Dispatch(r.Context(), notification)
```

The current `internal/notify` contract says `Dispatch` always returns nil and
logs backend errors via `slog.Error`. The HTTP handler must therefore return 200
after a valid request even if one or more backends fail.

If `Dispatch` returns a non-nil error in the future, treat it as a server error
and return 500.

#### `POST /notify` Testable Conditions

- **SERVER-NOTIFY-1** Missing `Authorization` returns 401.
- **SERVER-NOTIFY-2** Malformed `Authorization` returns 401.
- **SERVER-NOTIFY-3** Wrong bearer token returns 401.
- **SERVER-NOTIFY-4** Correct bearer token allows the request to reach the
  handler.
- **SERVER-NOTIFY-5** Malformed JSON returns 400.
- **SERVER-NOTIFY-6** Missing `event` returns 400.
- **SERVER-NOTIFY-7** Blank `event` returns 400.
- **SERVER-NOTIFY-8** Unknown `event` returns 422.
- **SERVER-NOTIFY-9** Valid `complete` dispatches
  `notify.Notification{Event: notify.EventComplete, ...}`.
- **SERVER-NOTIFY-10** Valid `waiting` dispatches
  `notify.Notification{Event: notify.EventWaiting, ...}`.
- **SERVER-NOTIFY-11** `agent` is trimmed before dispatch.
- **SERVER-NOTIFY-12** `agent` longer than 64 chars is truncated before dispatch.
- **SERVER-NOTIFY-13** `message` is trimmed before dispatch.
- **SERVER-NOTIFY-14** `message` longer than 512 chars is truncated before
  dispatch.
- **SERVER-NOTIFY-15** A valid request returns JSON with `ok: true` and the
  accepted event.
- **SERVER-NOTIFY-16** Backend failures logged by the dispatcher still produce
  HTTP 200 for a valid request.
- **SERVER-NOTIFY-17** The handler does not log the API key or full message
  content.

### `GET /health`

Returns liveness information. No authentication is required.

#### Response

```json
{
  "ok": true,
  "version": "v0.1.0",
  "uptime_seconds": 11640
}
```

| Field | Type | Notes |
|---|---|---|
| `ok` | bool | Always true while reachable. |
| `version` | string | Use the CLI version string unless build-time version injection is added. |
| `uptime_seconds` | integer | Seconds since the server started, rounded down. |

The current CLI version constant is `v0.1.0`; keep the leading `v` unless the
version contract is changed everywhere.

#### `GET /health` Testable Conditions

- **SERVER-HEALTH-1** `GET /health` returns 200 without an `Authorization`
  header.
- **SERVER-HEALTH-2** The response is valid JSON.
- **SERVER-HEALTH-3** `ok` is `true`.
- **SERVER-HEALTH-4** `version` is non-empty and matches the configured server
  version.
- **SERVER-HEALTH-5** `uptime_seconds` is an integer and never negative.
- **SERVER-HEALTH-6** `uptime_seconds` increases or stays equal across two
  sequential requests.

---

## Auth Middleware

Auth belongs in `internal/server/middleware.go`.

```go
func AuthMiddleware(key string, next http.Handler) http.Handler
```

Apply it only to `POST /notify`. Do not apply it to `GET /health`.

Validation:

1. Read `Authorization`.
2. Require exactly `Bearer <token>` semantics after trimming surrounding
   whitespace. Missing or malformed headers return 401.
3. Compare `<token>` with the configured key using `subtle.ConstantTimeCompare`.
4. Mismatch returns 401.
5. Match calls `next.ServeHTTP`.

The 401 body uses the standard error envelope:

```json
{
  "ok": false,
  "error": "unauthorized"
}
```

Do not reveal whether the header was missing, malformed, or wrong. Do not set
`WWW-Authenticate`; it is unnecessary for this CLI API.

### Auth Testable Conditions

- **SERVER-AUTH-1** `GET /health` does not require auth.
- **SERVER-AUTH-2** `POST /notify` without auth returns 401 and never dispatches.
- **SERVER-AUTH-3** `POST /notify` with `Authorization: Bearer wrong` returns
  401 and never dispatches.
- **SERVER-AUTH-4** `POST /notify` with the exact configured key reaches the
  next handler.
- **SERVER-AUTH-5** All auth failures return the same error string:
  `unauthorized`.

---

## Dispatcher Integration

The dispatcher is defined by `docs/specs/notify.md` and currently has this API:

```go
func notify.NewDispatcher(backends []notify.Backend) *notify.Dispatcher
func (d *notify.Dispatcher) Dispatch(ctx context.Context, n notify.Notification) error
```

Do not duplicate or change dispatcher behavior in `internal/server`. The server
only validates HTTP input, creates a `notify.Notification`, and calls
`Dispatch(r.Context(), n)`.

Dispatcher backend errors are logged by `internal/notify` at `slog.Error` with:

- message: `backend fire error`
- `backend`
- `event`
- `err`

The server must not add a second log entry for the same backend error.

### Dispatcher Integration Testable Conditions

- **SERVER-DISPATCH-1** The handler calls `Dispatch` exactly once for each valid
  `/notify` request.
- **SERVER-DISPATCH-2** The handler passes `r.Context()` into `Dispatch`.
- **SERVER-DISPATCH-3** The handler passes the sanitized notification values to
  `Dispatch`.
- **SERVER-DISPATCH-4** The handler does not call `Dispatch` for 400, 401, or 422
  requests.

---

## Backend Construction

Concrete toast and sound backends are planned under `internal/notify`, one file
per platform with Go build tags. When implemented, they must satisfy the
`notify.Backend` interface and the backend contract in `docs/specs/notify.md`.

`internal/cli/start.go` owns backend construction from config. It should create
the slice:

```go
backends := []notify.Backend{
    // toast backend if implemented
    // sound backend if implemented
}
dispatcher := notify.NewDispatcher(backends)
```

If concrete backends are not implemented yet, an empty backend slice is valid.
The HTTP server can still accept notifications and dispatch no-op successfully.

Do not add runtime `runtime.GOOS` switches for OS-specific backend behavior.
Use build-tagged files for concrete backend implementations.

### Backend Construction Testable Conditions

- **SERVER-BACKEND-1** `chime start` can run with an empty backend slice.
- **SERVER-BACKEND-2** Disabled backends are either omitted from the slice or
  return `false` from `Supports` for all events.
- **SERVER-BACKEND-3** Event filtering is handled by backend `Supports`, not by
  the HTTP handler.

---

## Graceful Shutdown

Foreground server shutdown is triggered by `SIGINT`, `SIGTERM`, or context
cancellation in tests.

Shutdown behavior:

1. Stop accepting new connections.
2. Allow in-flight requests to finish for up to 5 seconds.
3. After 5 seconds, close remaining connections.
4. Log shutdown signal and shutdown completion.
5. Return nil from the foreground command unless startup failed.

The dispatcher does not need separate shutdown handling. Backend implementations
are responsible for their own per-call timeouts as described in
`docs/specs/notify.md`.

### Shutdown Testable Conditions

- **SERVER-SHUTDOWN-1** Canceling the server context causes `Start` to return.
- **SERVER-SHUTDOWN-2** Shutdown uses a 5-second timeout.
- **SERVER-SHUTDOWN-3** After shutdown, new connections to the bound address fail.
- **SERVER-SHUTDOWN-4** A SIGTERM sent to `chime start --foreground` exits 0.
- **SERVER-SHUTDOWN-5** The foreground command prints `Server stopped.` after a
  graceful shutdown.

---

## Logging

Use `log/slog` with the default logger. The CLI/startup layer configures the
default logger once before the server begins serving. Other packages call
`slog.Info`, `slog.Debug`, `slog.Warn`, or `slog.Error` directly.

### Log Destination

For the foreground MVP, server logs should go to the log file. Avoid writing
routine server logs to stdout, because stdout is reserved for user-facing CLI
output such as generated keys and startup/shutdown lines.

If the log file cannot be opened, return an error from startup. This aligns with
`docs/specs/cli.md` condition START-19 and avoids silently losing server logs.

Log file path resolution:

1. `--log` flag, if non-empty.
2. `cfg.Log.File`, if non-empty.
3. `paths.LogFile()`.

Create the parent directory with mode `0755` before opening the file. Open the
log file in append mode, creating it with mode `0600`.

### Log Level

Use `cfg.Log.Level` with supported values:

- `debug`
- `info`
- `warn`
- `error`

Invalid values should return a startup error.

### What To Log

| Event | Level | Fields |
|---|---|---|
| Server started | info | `bind` |
| Shutdown signal/context cancellation | info | none |
| Shutdown complete | info | none |
| Shutdown timeout/error | warn | `err` |
| Panic recovered from handler | error | `err` |
| Backend failure | handled by `internal/notify` | do not duplicate |

At `debug` level, request logging may include method, path, and response status.
At `info` level, do not log every normal request.

Never log:

- API keys.
- Authorization headers.
- Full request bodies.
- `message` field content.

### Logging Testable Conditions

- **SERVER-LOG-1** Startup creates the log file if it does not exist.
- **SERVER-LOG-2** The log file is created with mode `0600`.
- **SERVER-LOG-3** Invalid log levels return a startup error.
- **SERVER-LOG-4** Routine server logs do not appear on stdout.
- **SERVER-LOG-5** `/notify` logging does not include the API key, authorization
  header, full request body, or message text.

---

## Error Handling Contract

| Scenario | Behavior |
|---|---|
| Config not loaded | `chime start` returns an error. |
| Config file missing | `config.Load` creates defaults before `start` runs. |
| API key missing from config | `start` generates and saves one before serving. |
| Invalid bind address | Startup returns error, exit 1. |
| Port already in use | Startup returns error, exit 1. |
| Invalid log level | Startup returns error, exit 1. |
| Log file parent cannot be created | Startup returns error, exit 1. |
| Log file cannot be opened | Startup returns error, exit 1. |
| Backend binary not found | Backend returns error; dispatcher logs; HTTP response remains 200. |
| All backends fail | Same as above; HTTP response remains 200. |
| Panic in handler | Recovery middleware logs error and returns HTTP 500. |
| Invalid JSON body | HTTP 400; do not dispatch. |
| Unknown event type | HTTP 422; do not dispatch. |
| Unauthorized request | HTTP 401; do not dispatch. |

Command files must return errors. Only `cmd/chime/main.go` writes errors to
stderr and exits.

---

## Package Layout

Target server-related layout:

```text
internal/server/
├── server.go       # Server struct, New, Start, Shutdown, route registration
├── handler.go      # notify and health handlers
├── middleware.go   # auth, max-body, panic recovery, optional request logging
└── response.go     # small JSON response helpers, if useful
```

Planned but separate packages/files:

```text
internal/notify/
├── backend.go
├── dispatcher.go
├── toast_darwin.go
├── toast_linux.go
├── toast_windows.go
├── sound_darwin.go
├── sound_linux.go
└── sound_windows.go

assets/
├── sounds/
│   ├── complete.aiff
│   └── waiting.aiff
└── embed.go

internal/client/
└── client.go

internal/daemon/
├── pid.go
└── daemon.go
```

Do not create `internal/daemon` as part of the foreground server implementation
unless the task explicitly includes background mode.

### Handler Registration

Using Go 1.22 method-aware patterns is acceptable if the project Go version
supports them:

```go
mux := http.NewServeMux()
mux.Handle("POST /notify", panicRecoveryMiddleware(
    AuthMiddleware(cfg.Auth.Key,
        maxBodyMiddleware(1<<20, http.HandlerFunc(s.notifyHandler)))))
mux.Handle("GET /health", panicRecoveryMiddleware(
    http.HandlerFunc(s.healthHandler)))
```

Panic recovery should wrap both handlers. Auth should be applied only to
`POST /notify`. Max-body limiting should apply only to endpoints that read a
body.

---

## Suggested Test Structure

Add tests close to behavior:

```text
internal/server/
├── server_test.go       # route registration, timeouts, Start/Shutdown
├── handler_test.go      # /notify and /health behavior
└── middleware_test.go   # auth, max body, panic recovery
```

For CLI startup behavior, use `internal/cli` tests with a temporary config path
and ephemeral TCP port. Do not use the user's real config file in tests.

Useful testing patterns:

- Use `httptest` for handler and middleware tests.
- Use a fake dispatcher or a dispatcher with fake backends for dispatch tests.
- Use `t.Setenv("CHIME_CONFIG", tempConfigPath)` or `--config` to isolate config.
- Use `127.0.0.1:0` for ephemeral port tests when possible.
- Use short contexts and channels to test shutdown without sleeping for the full
  timeout.

---

## Implementation Checklist

- Add `internal/server` package and tests.
- Wire `internal/cli/start.go` to the server package.
- Resolve bind/log values through existing config helpers and paths.
- Generate/save first-run API key with existing `config.GenerateKey` and
  `config.Save`.
- Preserve existing error bubbling through `main.go`.
- Keep daemon/PID behavior out of the server MVP.
- Run `goimports -w .`, `golangci-lint run`, and `go test ./...` before marking
  implementation complete.
