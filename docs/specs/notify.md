# `internal/notify` — Dispatch Package Spec

> Covers `backend.go` and `dispatcher.go` only.  
> OS-specific backend implementations (`toast_*.go`, `sound_*.go`) are out of scope.

---

## 1. Types

### 1.1 `Event`

```go
type Event string

const (
    EventComplete Event = "complete"
    EventWaiting  Event = "waiting"
)
```

- `Event` is a typed string, not an iota enum, so new event types can be added without breaking serialization.
- The dispatcher assumes it only receives known `Event` values. Validation of unknown event strings is the server handler's responsibility (returns HTTP 422).

---

### 1.2 `Notification`

```go
type Notification struct {
    Event   Event
    Agent   string // may be empty
    Message string // may be empty
}
```

- `Event` is required and must be a known constant. All other fields are optional.
- The struct is intentionally minimal. No ID, no timestamp — the dispatcher is fire-and-forget; it does not need to track or deduplicate notifications.

---

### 1.3 `Backend`

```go
type Backend interface {
    Name()     string
    Supports(event Event) bool
    Fire(n Notification) error
}
```

**`Name() string`**

Returns a short, stable identifier for the backend (e.g. `"toast"`, `"sound"`). Used only in log messages. Must be non-empty.

**`Supports(event Event) bool`**

Reports whether this backend should fire for the given event. The dispatcher calls this before each `Fire` — backends that return `false` are skipped entirely.

`Supports` handles two orthogonal concerns collapsed into one method:

- **Event filtering** — is this event type handled at all? (e.g. sound may be configured for `complete` only)
- **Enabled state** — is the backend active at all? A disabled backend should always return `false` regardless of event.

Rationale: keeping this in `Supports` means the dispatcher remains simple (one gate, no separate enabled check) and backends remain self-contained. The concrete implementation reads from the config values it was constructed with — it does not call into config at runtime.

**`Fire(n Notification) error`**

Executes the notification action. Must be safe to call concurrently with other backends. Returns `nil` on success, a descriptive error on failure.

`Fire` must not hang indefinitely. Each implementation is responsible for enforcing its own timeout (see §3.4).

---

## 2. `Dispatcher`

### 2.1 Definition

```go
type Dispatcher struct {
    backends []Backend
}
```

`backends` is the ordered list of backends to fan out to. Order has no semantic significance — all enabled backends fire concurrently.

---

### 2.2 Constructor

```go
func NewDispatcher(backends []Backend) *Dispatcher
```

- Accepts the fully-constructed backend list. The caller (CLI wiring in `internal/cli`) is responsible for building backends from config and passing them in.
- The dispatcher does not read config directly.
- `backends` may be empty; `Dispatch` on an empty dispatcher is a no-op that returns `nil`.
- `nil` entries in `backends` are not permitted. Callers must not pass them.

**Testable conditions:**

- `NewDispatcher(nil)` and `NewDispatcher([]Backend{})` both construct a valid, no-op dispatcher.
- `NewDispatcher` stores backends in the order provided (observable in tests via `Name()`).

---

### 2.3 `Dispatch`

```go
func (d *Dispatcher) Dispatch(ctx context.Context, n Notification) error
```

**Behavior:**

1. Iterates `d.backends`. For each backend where `b.Supports(n.Event)` returns `true`, launches `b.Fire(n)` in a goroutine.
2. Waits for all launched goroutines to complete.
3. Any error returned by `Fire` is logged via `slog.Error` with the backend name, event, and error. It is not propagated to the caller.
4. Always returns `nil`.

**Return value rationale:** The HTTP handler must respond promptly regardless of backend failures. Agent hook scripts do not check the notification result. Returning `nil` unconditionally keeps call sites clean and matches the fire-and-forget model.

**Context:** `ctx` is threaded through primarily to support cancellation by the HTTP server on shutdown. Backends should respect it where possible (see §3.4), but `Dispatch` itself does not enforce a deadline — it waits for all goroutines to finish naturally (or time out internally). The context is not used to short-circuit `wg.Wait`.

**Testable conditions:**

- A backend whose `Supports` returns `false` for the notification event must not have `Fire` called.
- A backend whose `Supports` returns `true` must have `Fire` called exactly once per `Dispatch` call.
- An error from `Fire` must not cause `Dispatch` to return a non-nil error.
- An error from `Fire` must be observable via the `slog` output (testable with a custom `slog.Handler`).
- All backends fire concurrently: a slow backend must not block other backends from completing.
- `Dispatch` with zero matching backends returns `nil` immediately.
- `Dispatch` waits for all goroutines before returning (no goroutine leak after return).

---

## 3. Backend Implementation Contract

This section defines what any concrete backend (`ToastBackend`, `SoundBackend`, etc.) must comply with. It is not part of the interface definition but is a required behavioral contract for all implementations.

### 3.1 `Name`

- Must return the same value on every call (pure, no side effects).
- Should be lowercase, no spaces (e.g. `"toast"`, `"sound"`).

### 3.2 `Supports`

- Must be pure — no I/O, no side effects. The dispatcher may call it multiple times.
- Must return `false` if the backend is disabled in config, regardless of the event argument.
- Must return `false` for any event not in the backend's configured event list.
- Must return `true` only if both conditions hold: the backend is enabled AND the event is in its event list.

**Testable conditions:**

- A disabled backend returns `false` for all events.
- An enabled backend with `events: [complete]` returns `true` for `EventComplete` and `false` for `EventWaiting`.
- An enabled backend with `events: [complete, waiting]` returns `true` for both.
- An enabled backend with an empty event list returns `false` for all events.

### 3.3 `Fire`

- Must not call `os.Exit`, `log.Fatal`, or write to `os.Stderr` directly.
- Must return a descriptive error rather than panicking on failure.
- Return `nil` only if the notification was successfully dispatched (the OS command was invoked without error). Whether the user actually saw the toast is outside the contract.
- May perform `exec.Command` calls or other OS-level work.

### 3.4 Timeout Requirement

- `Fire` must not block indefinitely. Each implementation must enforce a per-call timeout, independent of the caller.
- Recommended default: **5 seconds**. Implementations should use `exec.CommandContext` with a derived context:

```go
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
cmd := exec.CommandContext(ctx, ...)
```

- If the timeout elapses, `Fire` must return an error (the context cancellation error is acceptable).
- The 5-second value should be a named constant in the implementation file, not a magic number.

**Testable conditions:**

- A backend whose underlying OS command times out returns a non-nil error before the deadline plus a small margin.

---

## 4. Log Output

The dispatcher logs backend errors at `slog.Error` level with the following fields:

| Field     | Key       | Value                        |
|-----------|-----------|------------------------------|
| message   | (message) | `"backend fire error"`       |
| backend   | `backend` | `b.Name()`                   |
| event     | `event`   | `string(n.Event)`            |
| error     | `err`     | the error returned by `Fire` |

Example (rendered as text):

```
level=ERROR msg="backend fire error" backend=sound event=complete err="exec: afplay exited with status 1"
```

No other `slog` calls are made by the dispatcher. Backends may log debug information internally but must not log their own errors — that is the dispatcher's job.

---

## 5. File Layout

These are the only files in scope for this spec:

```
internal/notify/
├── backend.go      # Event, Notification, Backend interface
└── dispatcher.go   # Dispatcher struct, NewDispatcher, Dispatch
```

OS-specific backend files (`toast_darwin.go`, `sound_linux.go`, etc.) satisfy the `Backend` interface defined here and are specced separately.

---

## 6. Summary of Public API

```go
// backend.go

type Event string

const (
    EventComplete Event = "complete"
    EventWaiting  Event = "waiting"
)

type Notification struct {
    Event   Event
    Agent   string
    Message string
}

type Backend interface {
    Name()     string
    Supports(event Event) bool
    Fire(n Notification) error
}

// dispatcher.go

type Dispatcher struct { /* unexported fields */ }

func NewDispatcher(backends []Backend) *Dispatcher
func (d *Dispatcher) Dispatch(ctx context.Context, n Notification) error
```

No other exported symbols belong in these two files.