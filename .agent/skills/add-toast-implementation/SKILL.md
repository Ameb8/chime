---
name: add-toast-implementation
description: Implement the OS-agnostic integration layer for Chime's toast notification backend. Use when adding or revising shared toast backend setup, config mapping, notification title/body formatting, event filtering, tests, or dispatcher/server wiring that applies to all concrete platform implementations without writing darwin/linux/windows command execution code.
---

# Add Toast Implementation

## Required Context

Read these before editing code:

- `AGENTS.md`
- `docs/design.md`
- `docs/specs/notify.md`
- Any toast-specific spec under `docs/specs/` if present
- Existing `internal/notify`, `internal/config`, and server/CLI wiring files relevant to backend construction

Treat `docs/design.md` as the architecture source of truth and `docs/specs/notify.md` as the dispatcher/backend contract source of truth. If they conflict, stop and surface the conflict before coding.

## Scope

Implement shared, OS-neutral toast-backend behavior only. Do not implement platform-specific notification commands in this skill pass.

In scope:

- Toast backend construction from already-resolved config values
- Enabled/event filtering through `Supports`
- Shared notification title/body formatting for `complete` and `waiting`
- Interfaces or adapters that concrete platform files can satisfy later
- Server/CLI wiring needed to include the toast backend in the dispatcher
- Unit tests using fakes, not real OS notification APIs

Out of scope unless explicitly requested:

- `osascript`, `notify-send`, PowerShell, BurntToast, or other concrete command execution
- `runtime.GOOS` switches
- Sound backend behavior
- Real desktop notification integration tests
- Daemon/service work

## Implementation Rules

- Keep all notification logic under `internal/notify`; keep `cmd/chime/main.go` as entrypoint only.
- Do not make the dispatcher read config. Build backends outside the dispatcher and pass them to `notify.NewDispatcher`.
- Prefer a typed options struct over raw config access inside `internal/notify` when that keeps package boundaries clean.
- Make `Supports(event)` pure: no I/O, no mutation, no logging.
- Treat disabled backends and empty event lists as supporting no events.
- Copy caller-owned slices/maps in constructors so later config mutation cannot change backend behavior.
- Keep `Fire` responsible only for formatting notification content and delegating delivery to an injected/shared sender. It should return descriptive errors; do not log backend errors from `Fire`.
- Leave backend error logging to `Dispatcher.Dispatch`.
- Enforce the notify backend contract: no `os.Exit`, no `log.Fatal`, no direct stderr writes.
- Do not use `runtime.GOOS`; concrete platform implementations belong in `toast_darwin.go`, `toast_linux.go`, `toast_windows.go`, etc. with build tags.

## Preferred Shape

Use the existing codebase patterns first. If no pattern exists, prefer a small structure like:

```go
type ToastOptions struct {
	Enabled bool
	Events  []Event
}

type toastSender interface {
	Show(title, body string) error
}
```

Then make a toast backend that:

- Has `Name() string` return `"toast"`.
- Precomputes supported events at construction time.
- Formats title and body once in shared code so each platform implementation gets identical user-facing text.
- Returns an error for an unsupported or unformattable event in `Fire`, even though callers normally call `Supports` first.
- Delegates actual notification delivery to a sender abstraction so tests never call OS notification tools.

Adjust names and exportedness to match the repository's current style. Export only what must be constructed outside the package.

## Notification Text

Follow `docs/design.md` for user-facing content.

- Title should identify Chime and include the agent when provided.
- Body should distinguish `complete` from `waiting`.
- Include `Notification.Message` only when it is non-empty.
- Keep formatting deterministic and centralized in the shared toast code.
- Do not duplicate title/body construction in platform-specific files.

If the exact title/body wording is underspecified, choose a minimal helper with table-driven tests and avoid spreading literals across multiple files.

## Wiring

When server construction exists, wire the toast backend where the dispatcher backend list is assembled. Keep this construction separate from HTTP request handling.

When server construction does not exist yet, add only the toast package pieces and tests. Do not create speculative server APIs just to wire the backend.

## Tests

Add focused unit tests for shared behavior:

- `Supports` returns false when disabled.
- `Supports` returns true only for configured events.
- Empty event lists support no events.
- Constructor copies event lists.
- `Fire` passes the expected title/body to the sender for `complete`.
- `Fire` passes the expected title/body to the sender for `waiting`.
- Agent and message are optional and format cleanly when empty.
- `Fire` propagates sender errors without logging.
- No test runs a real OS notification command.

Use fake senders and table-driven tests for formatting. Avoid sleeps unless testing timeout behavior in a concrete platform file.

## Validation

Before finishing, run:

```sh
goimports -w .
golangci-lint run
go test ./...
```

If a required command fails because of pre-existing unrelated work, report that clearly and include the failing output summary.
