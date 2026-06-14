---
name: add-sound-implementation
description: Implement the OS-agnostic integration layer for Chime's sound notification backend. Use when adding or revising shared sound backend setup, config mapping, embedded default sound handling, event filtering, tests, or wiring that applies to all concrete platform implementations without writing darwin/linux/windows command execution code.
---

# Add Sound Implementation

## Required Context

Read these before editing code:

- `AGENTS.md`
- `docs/design.md`
- `docs/specs/notify.md`
- Any sound-specific spec under `docs/specs/` if present
- Existing `internal/notify`, `internal/config`, `assets`, and server/CLI wiring files relevant to backend construction

Treat `docs/design.md` as the architecture source of truth and `docs/specs/notify.md` as the dispatcher/backend contract source of truth. If they conflict, stop and surface the conflict before coding.

## Scope

Implement shared, OS-neutral sound-backend behavior only. Do not implement platform-specific playback commands in this skill pass.

In scope:

- Sound backend construction from already-resolved config values
- Enabled/event filtering through `Supports`
- Event-to-sound selection for `complete` and `waiting`
- Shared fallback behavior for embedded default sounds
- Interfaces or adapters that concrete platform files can satisfy later
- Server/CLI wiring needed to include the sound backend in the dispatcher
- Unit tests using fakes, not real audio or OS commands

Out of scope unless explicitly requested:

- `afplay`, `paplay`, PowerShell, `notify-send`, or other concrete command execution
- `runtime.GOOS` switches
- Real sound assets beyond wiring to the existing `assets/embed.go` pattern
- Toast backend behavior
- Daemon/service work

## Implementation Rules

- Keep all notification logic under `internal/notify`; keep `cmd/chime/main.go` as entrypoint only.
- Do not make the dispatcher read config. Build backends outside the dispatcher and pass them to `notify.NewDispatcher`.
- Prefer a typed options struct over raw config access inside `internal/notify` when that keeps package boundaries clean.
- Make `Supports(event)` pure: no I/O, no mutation, no logging.
- Treat disabled backends and empty event lists as supporting no events.
- Copy caller-owned slices/maps in constructors so later config mutation cannot change backend behavior.
- Keep `Fire` responsible only for selecting the sound and delegating playback to an injected/shared player. It should return descriptive errors; do not log backend errors from `Fire`.
- Leave backend error logging to `Dispatcher.Dispatch`.
- Enforce the notify backend contract: no `os.Exit`, no `log.Fatal`, no direct stderr writes.
- Do not use `runtime.GOOS`; concrete platform implementations belong in `sound_darwin.go`, `sound_linux.go`, `sound_windows.go`, etc. with build tags.

## Preferred Shape

Use the existing codebase patterns first. If no pattern exists, prefer a small structure like:

```go
type SoundOptions struct {
	Enabled       bool
	Events        []notify.Event // or []Event inside package notify
	CompleteSound string
	WaitingSound  string
}

type soundPlayer interface {
	Play(path string) error
}
```

Then make a sound backend that:

- Has `Name() string` return `"sound"`.
- Precomputes supported events at construction time.
- Uses configured paths when set.
- Uses embedded defaults for event sounds when custom paths are empty.
- Returns an error for an unsupported or unresolvable sound in `Fire`, even though callers normally call `Supports` first.
- Delegates actual playback to a player abstraction so tests never invoke the OS.

Adjust names and exportedness to match the repository's current style. Export only what must be constructed outside the package.

## Embedded Defaults

Follow `docs/design.md` for embedded sounds:

- Sound files are embedded once in `assets/embed.go`.
- Notify code imports/references package-level values from `assets`; it must not add a second `go:embed`.
- If playback APIs require a filesystem path, write embedded bytes to a cache/data location through `internal/paths` or another established helper, and make that behavior testable.
- Do not invent hardcoded user paths.

## Wiring

When server construction exists, wire the sound backend where the dispatcher backend list is assembled. Keep this construction separate from HTTP request handling.

When server construction does not exist yet, add only the sound package pieces and tests. Do not create speculative server APIs just to wire the backend.

## Tests

Add focused unit tests for shared behavior:

- `Supports` returns false when disabled.
- `Supports` returns true only for configured events.
- Empty event lists support no events.
- Constructor copies event lists and path data.
- `Fire` selects the custom complete/waiting path when configured.
- `Fire` falls back to embedded defaults when custom paths are empty.
- `Fire` propagates player errors without logging.
- No test runs a real OS sound command.

Use fake players and temporary directories. Avoid sleeps unless testing timeout behavior in a concrete platform file.

## Validation

Before finishing, run:

```sh
goimports -w .
golangci-lint run
go test ./...
```

If a required command fails because of pre-existing unrelated work, report that clearly and include the failing output summary.
