# `chime notify` - CLI Command Design Spec

> Implementation target for the `chime notify` client command.
>
> Read alongside:
>
> - `docs/design.md` - architecture, package responsibilities, HTTP API, and
>   current project scope.
> - `docs/specs/cli.md` - command behavior, flags, output streams, and exit
>   codes.
> - `docs/specs/config.md` - typed config, `config.Resolve`, and
>   flag/env/config precedence.
> - `docs/specs/server.md` - authoritative `/notify` endpoint behavior.
> - `docs/specs/notify.md` - `internal/notify` dispatch package; this is a
>   separate server-side package spec and must not be confused with this command
>   spec.

---

## Status And Scope

The current `internal/cli/notify.go` command is wired into the root command but
returns `notImplemented("notify")`. This spec defines the target behavior for
implementing that command and the HTTP client it uses.

### In Scope

- Implement `chime notify --event <event> [flags]`.
- Add `internal/client` for the HTTP client used by the command.
- Resolve `--server` and `--key` through `config.Resolve`.
- Validate command input before making network requests.
- Send authenticated `POST /notify` requests to the configured server.
- Map validation, auth, HTTP, timeout, and connection failures to the exit codes
  in `docs/specs/cli.md`.
- Add command and client tests.

### Out Of Scope

- Server handler implementation. See `docs/specs/server.md`.
- Toast, sound, dispatcher, and backend behavior. See `docs/specs/notify.md`.
- Hook snippet generation. See `chime install`.
- Background daemonizing or service installation.
- Writing to agent config files.

---

## User-Facing Contract

```text
chime notify --event <event> [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--event` | string | none | Required. Event type: `complete` or `waiting`. |
| `--agent` | string | `""` | Agent name, e.g. `claude-code`, `codex`, or `aider`. |
| `--message` | string | `""` | Optional human-readable detail from the agent. |
| `--server` | string | resolved | Server base URL. |
| `--key` | string | resolved | API key. |

The command accepts no positional arguments.

### Output

- On success, print nothing to stdout and nothing to stderr.
- On failure, return an error from `RunE`; `cmd/chime/main.go` is the only place
  that writes the error to stderr and exits.
- Do not use `slog` for command output. `chime notify` is a client command, not
  server operational logging.

### Exit Codes

| Code | Scenario |
|---|---|
| `0` | Notification accepted by the server. |
| `1` | Bad command usage, invalid event, missing server URL, invalid server URL, malformed successful/error response handling that cannot be classified more specifically, or other general local error. |
| `2` | API key is missing or the server returns `401 Unauthorized`. |
| `3` | Server is unreachable, request times out, DNS/connect/TLS transport fails, or the server returns a non-auth HTTP error. |

`chime notify` must use `*exitcode.Error` for non-1 exit codes so `main.go`
can preserve the intended code.

---

## Resolution And Validation

### Config Access

The root command loads config once and passes `**config.Config` into
`newNotifyCmd`. `notify.go` must use that loaded config value. It must not call
Viper directly.

If the config pointer is nil when the command runs, return a general error.

### Server URL Resolution

Resolve the server URL exactly as:

```go
serverURL := config.Resolve(opts.server, "CHIME_SERVER", cfg.Client.Server)
```

Precedence:

1. `--server`
2. `CHIME_SERVER`
3. `client.server`

If the resolved server URL is empty, return a local configuration error and exit
1. Do not derive a default from `server.bind`; `client.server` is the canonical
client target.

The resolved server URL must include an HTTP or HTTPS scheme and a host. Invalid
URLs return exit 1 before any network request is attempted.

Trailing slashes are allowed. The client appends `/notify` to the base URL
without producing a double slash.

### API Key Resolution

Resolve the API key exactly as:

```go
apiKey := config.Resolve(opts.key, "CHIME_KEY", cfg.Auth.Key)
```

Precedence:

1. `--key`
2. `CHIME_KEY`
3. `auth.key`

If the resolved key is empty, return `&exitcode.Error{Code:
exitcode.AuthFailure, ...}`. Do not make a network request without a key.

### Event Validation

Only these event values are valid:

- `complete`
- `waiting`

An empty or unknown `--event` value is a local usage error and exits 1. This is
separate from the server's HTTP 422 behavior, which protects the server API from
unknown remote clients.

The command should use `notify.EventComplete` and `notify.EventWaiting` for
validation constants rather than duplicating raw strings where practical.

### Agent And Message

`--agent` and `--message` are optional and may be empty.

The command does not trim, truncate, redact, or otherwise transform these values
before sending. Server-side sanitization is specified in `docs/specs/server.md`.

---

## `internal/client`

Add a package:

```text
internal/client/
`-- client.go
```

Suggested public API:

```go
package client

import (
    "context"
    "net/http"
    "time"
)

const DefaultTimeout = 5 * time.Second

type Client struct {
    // unexported fields
}

type Options struct {
    BaseURL    string
    APIKey     string
    HTTPClient *http.Client
}

type Notification struct {
    Event   string
    Agent   string
    Message string
}

func New(opts Options) (*Client, error)
func (c *Client) Notify(ctx context.Context, n Notification) error
```

This package owns HTTP details. The CLI package owns Cobra flags, config
resolution, and conversion from errors to exit codes.

### Constructor

`New` validates:

- `BaseURL` is non-empty.
- `BaseURL` parses successfully.
- Scheme is `http` or `https`.
- Host is non-empty.
- `APIKey` is non-empty.

If `HTTPClient` is nil, construct one with `Timeout: DefaultTimeout`.

`New` should normalize and store the final endpoint URL as `<base>/notify`.

### Request

`Notify` sends:

```http
POST /notify HTTP/1.1
Authorization: Bearer <api-key>
Content-Type: application/json
Accept: application/json

{
  "event": "complete",
  "agent": "claude-code",
  "message": "Task finished"
}
```

Payload fields:

| JSON field | Source |
|---|---|
| `event` | `Notification.Event` |
| `agent` | `Notification.Agent` |
| `message` | `Notification.Message` |

Use the standard library only. Do not add retry behavior; hook scripts should
return quickly and should use `|| true` if notification failure must not block
agent execution.

### Response Handling

The server response envelope is:

```json
{
  "ok": true,
  "event": "complete"
}
```

Error responses use:

```json
{
  "ok": false,
  "error": "unauthorized"
}
```

The client should read a bounded amount of response body data. A small limit such
as 64 KiB is sufficient.

Treat status codes as:

| Status | Client result |
|---|---|
| `200 OK` | Success if the response is valid enough to confirm acceptance. |
| `401 Unauthorized` | Auth failure. |
| Any other status | Server/connection failure. |

The client may include the server's short `error` string in the returned error
message, but it must never include the API key or `Authorization` header.

### Error Types

Define package-level sentinel errors or typed errors that allow `internal/cli`
to classify failures without string matching. At minimum, expose categories for:

- invalid local input/configuration
- missing/invalid auth
- unreachable server or timeout
- non-401 HTTP status

Implementation can use `errors.Is` or a small typed error. The exact structure is
less important than avoiding brittle string matching in the CLI layer.

---

## CLI Wiring

`internal/cli/notify.go` should:

1. Define and bind the existing flags.
2. Keep `Args: cobra.NoArgs`.
3. Keep `cmd.MarkFlagRequired("event")` or perform equivalent validation in
   `RunE`.
4. In `RunE`, read the loaded config pointer.
5. Validate `--event`.
6. Resolve server URL and API key with `config.Resolve`.
7. Construct `client.New`.
8. Call `Notify` with `cmd.Context()`.
9. Return nil on success.
10. Convert classified client errors to the correct returned error type.

Suggested high-level shape:

```go
RunE: func(cmd *cobra.Command, _ []string) error {
    cfg := *cfgPtr
    if cfg == nil {
        return errors.New("config not loaded")
    }

    event, err := parseNotifyEvent(opts.event)
    if err != nil {
        return err
    }

    serverURL := config.Resolve(opts.server, "CHIME_SERVER", cfg.Client.Server)
    apiKey := config.Resolve(opts.key, "CHIME_KEY", cfg.Auth.Key)

    c, err := client.New(client.Options{
        BaseURL: serverURL,
        APIKey:  apiKey,
    })
    if err != nil {
        return classifyNotifySetupError(err)
    }

    err = c.Notify(cmd.Context(), client.Notification{
        Event:   string(event),
        Agent:   opts.agent,
        Message: opts.message,
    })
    return classifyNotifySendError(err)
}
```

Do not write directly to stdout or stderr from this command.

---

## Error Mapping

Use concise error messages intended for stderr via `main.go`.

| Scenario | Returned error behavior |
|---|---|
| Missing `--event` | General error, exit 1. |
| Unknown `--event` | General error, exit 1. |
| Positional argument present | Cobra args error, exit 1. |
| Config pointer nil | General error, exit 1. |
| Resolved server URL empty | General error, exit 1. |
| Resolved server URL invalid | General error, exit 1. |
| Resolved API key empty | `exitcode.AuthFailure`, exit 2. |
| HTTP 401 | `exitcode.AuthFailure`, exit 2. |
| Connection refused | `exitcode.ServerUnreachable`, exit 3. |
| DNS failure | `exitcode.ServerUnreachable`, exit 3. |
| Request timeout or context deadline | `exitcode.ServerUnreachable`, exit 3. |
| HTTP 400, 404, 405, 422, or 500 | `exitcode.ServerUnreachable`, exit 3. |
| Response body cannot be decoded on HTTP 200 | General error, exit 1. |

HTTP 422 should normally be unreachable because the CLI validates event values
locally. If it occurs, classify it as server-side request failure with exit 3.

---

## Security And Logging

- Do not log or print API keys.
- Do not include the `Authorization` header in errors.
- Do not include the full request body in errors.
- Do not use `slog` for normal command behavior.
- Avoid echoing `--message` in error messages; hook payloads may contain user or
  tool input.

---

## Testable Conditions

### Command Validation

- **NOTIFY-CMD-1** `chime notify` without `--event` exits 1 and prints an error
  to stderr through `main.go`.
- **NOTIFY-CMD-2** `chime notify --event unknown` exits 1 and does not make an
  HTTP request.
- **NOTIFY-CMD-3** `chime notify --event complete extra` exits 1.
- **NOTIFY-CMD-4** `chime notify --event complete` with no resolvable server URL
  exits 1 and does not make an HTTP request.
- **NOTIFY-CMD-5** `chime notify --event complete --server ://bad` exits 1 and
  does not make an HTTP request.
- **NOTIFY-CMD-6** `chime notify --event complete` with no resolvable API key
  exits 2 and does not make an HTTP request.

### Resolution Precedence

- **NOTIFY-RESOLVE-1** `--server` takes precedence over `CHIME_SERVER`.
- **NOTIFY-RESOLVE-2** `CHIME_SERVER` takes precedence over `client.server`.
- **NOTIFY-RESOLVE-3** `--key` takes precedence over `CHIME_KEY`.
- **NOTIFY-RESOLVE-4** `CHIME_KEY` takes precedence over `auth.key`.
- **NOTIFY-RESOLVE-5** Resolution is implemented through `internal/config`, not
  inline environment lookups in `internal/cli/notify.go`.

### HTTP Request

- **NOTIFY-HTTP-1** A successful command sends `POST <server>/notify`.
- **NOTIFY-HTTP-2** The request includes `Authorization: Bearer <key>`.
- **NOTIFY-HTTP-3** The request includes `Content-Type: application/json`.
- **NOTIFY-HTTP-4** `--event complete --agent claude-code` sends JSON containing
  `"event":"complete"` and `"agent":"claude-code"`.
- **NOTIFY-HTTP-5** `--message "done"` sends JSON containing
  `"message":"done"`.
- **NOTIFY-HTTP-6** A base URL with a trailing slash still sends to exactly one
  `/notify` path.

### Success And Failure

- **NOTIFY-RESULT-1** HTTP 200 with `ok: true` exits 0 with no stdout output.
- **NOTIFY-RESULT-2** HTTP 401 exits 2.
- **NOTIFY-RESULT-3** Connection refused exits 3.
- **NOTIFY-RESULT-4** Request timeout exits 3.
- **NOTIFY-RESULT-5** HTTP 500 exits 3.
- **NOTIFY-RESULT-6** No failure message includes the API key.
- **NOTIFY-RESULT-7** All command failures produce stderr output only through
  the root error path; command code itself does not write stderr.

---

## Suggested Test Structure

```text
internal/client/
`-- client_test.go       # URL validation, request construction, response/error classification

internal/cli/
`-- notify_test.go       # Cobra command validation, config resolution, exit-code errors
```

Use `httptest.Server` for success and HTTP-status tests. Use an unreachable
`127.0.0.1` port or a custom failing `RoundTripper` for transport failure tests.

Tests must use temporary config paths and must not read or write the user's real
config file.

---

## Implementation Checklist

- Add `internal/client`.
- Implement `newNotifyCmd` `RunE` behavior.
- Validate events against `internal/notify` event constants.
- Resolve `--server` and `--key` with `config.Resolve`.
- Map missing key and HTTP 401 to `exitcode.AuthFailure`.
- Map network/timeout/non-401 HTTP failures to `exitcode.ServerUnreachable`.
- Preserve silent success.
- Add client and CLI tests.
- Run `goimports -w .`, `golangci-lint run`, and `go test ./...` before marking
  implementation complete.
