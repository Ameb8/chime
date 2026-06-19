# `chime run` - CLI Command Design Spec

> Implementation target for running an arbitrary command and notifying the
> configured Chime server when that command finishes.
>
> Read alongside:
>
> - `docs/design.md` - architecture, package responsibilities, HTTP API, and
>   current project scope.
> - `docs/specs/cli.md` - command behavior, flags, output streams, and exit
>   codes.
> - `docs/specs/config.md` - typed config, `config.Resolve`, and
>   flag/env/config precedence.
> - `docs/specs/notify-command.md` - existing `chime notify` client behavior.
> - `docs/specs/server.md` - authoritative `/notify` endpoint behavior.

---

## Status And Scope

`chime run` wraps another command, mirrors that command's standard streams, and
sends a `complete` notification after the wrapped command exits.

The primary user-facing contract is:

```sh
chime run -- docker compose build --force-recreate
```

The `--` delimiter is required for every invocation. Everything after `--`
belongs to the wrapped command and must not be parsed as a Chime flag.

### In Scope

- Add a new top-level `chime run` command.
- Execute the wrapped command without invoking a shell.
- Attach the wrapped command's stdin, stdout, and stderr to Chime's own process
  so interactive and streaming commands behave normally.
- Preserve the wrapped command's exit code as the `chime run` process exit code.
- On Linux and macOS, preserve signal termination using shell-compatible exit
  codes such as `130` for `SIGINT`.
- Send a `complete` notification after the wrapped command exits.
- Include the wrapped command, exit code, success/failure status, and duration in
  the notification message.
- Resolve `--server` and `--key` using the same precedence as `chime notify`.
- Allow notification delivery failures without changing the wrapped command's
  exit code.
- Add tests for argument parsing, command execution, exit-code preservation, and
  notification payload behavior.

### Out Of Scope

- Running commands through a shell by default.
- Parsing shell syntax, pipelines, redirects, aliases, glob expansion, or shell
  builtins.
- Retrying notification delivery.
- Sending a notification before the command starts.
- Changing the server `/notify` API.
- Adding new notification event types for command success or failure.
- Capturing or uploading command output.
- Adding daemon, job control, or process supervision behavior beyond running one
  foreground child process.
- Full Windows signal semantics for `chime run`.

---

## User-Facing Contract

```text
chime run [flags] -- <command> [args...]
```

### Examples

```sh
chime run -- docker compose build --force-recreate
chime run --agent terminal -- make test
chime run --message "frontend build" -- npm run build
chime run --server http://192.168.1.20:7777 --key "$CHIME_KEY" -- go test ./...
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--agent` | string | executable basename | Agent/source name sent with the notification. |
| `--message` | string | `""` | Optional user-provided label included in the notification message. |
| `--server` | string | resolved | Server base URL. |
| `--key` | string | resolved | API key. |

`chime run` does not expose `--event`. The event is always `complete` because
the command sends one completion notification after the wrapped process exits.

### Required Delimiter

The implementation MUST support:

```sh
chime run -- docker compose build --force-recreate
```

The `--` delimiter is mandatory for every invocation. The implementation MUST
reject:

```sh
chime run docker compose build
```

The implementation MUST also reject:

```sh
chime run docker compose build --force-recreate
```

The error should tell the user to use:

```sh
chime run -- docker compose build --force-recreate
```

Rationale: without `--`, Cobra may treat wrapped command flags as Chime flags,
which is surprising and unsafe.

---

## Output Streams

### Wrapped Command Output

The wrapped command's streams MUST be attached directly:

```go
child.Stdin = os.Stdin
child.Stdout = os.Stdout
child.Stderr = os.Stderr
```

This preserves normal command behavior:

- Long-running output streams in real time.
- Interactive prompts can read from stdin.
- ANSI color output and progress displays are controlled by the wrapped command.
- Large output does not accumulate in Chime memory.

### Chime Output

On normal command completion, `chime run` SHOULD print no additional stdout.

If notification delivery fails after the command exits, `chime run` SHOULD print
a short warning to stderr:

```text
chime: notification failed: server unavailable: connection refused
```

That warning MUST NOT change the process exit code when the wrapped command ran.

If Chime fails before starting the wrapped command due to bad usage or invalid
configuration, it MUST return an error from `RunE`; `cmd/chime/main.go` remains
the only place that writes the error to stderr and exits.

---

## Exit Code Semantics

`chime run` has two distinct classes of failures:

1. Chime wrapper failures before the child process starts.
2. Wrapped command exit status after the child process starts.

### Before Child Start

If the wrapped command is not started, `chime run` exits according to the normal
Chime CLI exit-code table:

| Code | Scenario |
|---|---|
| `1` | Bad usage, no command provided, invalid local configuration, executable not found before start, or command start failure. |

Missing or invalid notification configuration MUST NOT prevent the wrapped
command from running. Notification configuration and delivery failures are
handled after the wrapped command exits and are reported as warnings only.

### After Child Start

If the wrapped command starts, the final `chime run` exit code MUST match the
wrapped command result:

| Wrapped command result | `chime run` exit code |
|---|---|
| exits `0` | `0` |
| exits `1` | `1` |
| exits `2` | `2` |
| exits `127` | `127` |
| exits with any valid process code | same code |
| terminates by signal on Linux/macOS | `128 + signal number`, for example `130` for `SIGINT` |

Notification success or failure MUST NOT override the wrapped command's exit
code once the command has started.

### Required Main-Process Support

The current `cmd/chime/main.go` prints any returned error before exiting. That
is correct for ordinary command errors but awkward for propagating an arbitrary
child exit code.

Implementation SHOULD add an exit-code-only sentinel in `internal/exitcode`,
for example:

```go
type SilentError struct {
    Code int
}

func (e *SilentError) Error() string { return "" }
```

`main.go` SHOULD detect this sentinel before printing to stderr and call
`os.Exit(e.Code)` without printing an empty or misleading error message.

Alternative: `runNotify` may call a package-level helper that returns an error
only when the child exit code is non-zero, but `main.go` still needs a way to
avoid printing a synthetic Chime error for normal wrapped command failures.

---

## Notification Semantics

### Event

`chime run` MUST send:

```json
{
  "event": "complete"
}
```

No new server event type is required.

### Agent

The `agent` field is the short source label rendered in toast titles as:

```text
chime - <agent>
```

The server currently truncates `agent` to 64 runes. Because the wrapped command
can be long, the full display command SHOULD NOT be used as the default `agent`
unless the product decision explicitly favors truncated titles.

Default agent:

```text
<executable basename>
```

For example, `chime run -- docker compose build --force-recreate` would send
`docker` as the agent and keep the full command in `message`.

If `--agent` is provided, send that value unchanged. The full command MUST still
be included in `message` regardless of the selected agent default.

### Message

The message SHOULD be a concise human-readable summary formatted from:

- Optional user label from `--message`.
- Display form of the wrapped command.
- Exit code.
- Success/failure status.
- Elapsed duration rounded to a human-readable precision.

Recommended success format:

```text
docker compose build --force-recreate completed successfully in 2m31s
```

Recommended failure format:

```text
docker compose build --force-recreate failed with exit code 17 after 2m31s
```

Recommended signal-termination format:

```text
docker compose build --force-recreate interrupted by SIGINT with exit code 130 after 12s
```

Recommended format with `--message`:

```text
frontend build: npm run build completed successfully in 48s
```

The command string MUST be display-only. It MUST NOT be executed through a shell.

### Duration

Duration starts immediately before `child.Start()` or `child.Run()` and ends
immediately after the wrapped command exits.

Recommended formatting:

- `<1s` for durations under one second.
- `Ns` for durations under one minute.
- `NmNs` for durations under one hour.
- `NhNm` for durations of one hour or more.

### Notification Timing

The notification MUST be attempted after the wrapped command exits and before
`chime run` exits.

The notification request uses the existing `internal/client` timeout. No extra
retry behavior is added.

---

## Config Resolution

`chime run` MUST use the same server/key resolution rules as `chime notify`.

### Server URL

Resolve exactly as:

```go
serverURL := config.Resolve(opts.server, "CHIME_SERVER", cfg.Client.Server)
```

Precedence:

1. `--server`
2. `CHIME_SERVER`
3. `client.server`

### API Key

Resolve exactly as:

```go
apiKey := config.Resolve(opts.key, "CHIME_KEY", cfg.Auth.Key)
```

Precedence:

1. `--key`
2. `CHIME_KEY`
3. `auth.key`

### Validation Timing

Required behavior:

- Validate command arguments before starting the child.
- Load config before starting the child, matching existing root command behavior.
- Do not perform a network preflight before starting the child.
- If server URL or API key is missing or invalid, still run the wrapped command.
- After the wrapped command exits, attempt notification setup and delivery.
- If notification setup or delivery fails, print a warning to stderr and
  preserve the wrapped command's exit code.

Rationale: `chime run` is a command wrapper first and a notifier second. Chime
misconfiguration must not block the user's build, test, or deployment command.

---

## Command Execution

### Execution Model

The wrapped command MUST be executed with:

```go
exec.CommandContext(cmd.Context(), argv[0], argv[1:]...)
```

The command MUST NOT be passed through `sh -c`, `zsh -c`, `cmd.exe /C`, or
PowerShell by default.

Rationale:

- Preserves argv exactly.
- Avoids shell injection risks.
- Avoids cross-platform shell differences.
- Makes `chime run -- docker compose build --force-recreate` behave like direct
  process execution.

### Working Directory

The child process inherits Chime's current working directory.

No `--cwd` flag is included in the initial scope.

### Environment

The child process inherits Chime's environment.

No environment mutation is included in the initial scope.

### Signals And Cancellation

Linux and macOS support is required. Windows signal semantics are out of scope
for the initial implementation.

Required behavior on Linux and macOS:

- If the wrapped command exits because of a signal, `chime run` MUST compute the
  shell-compatible exit code as `128 + signal number`.
- If the wrapped command exits because of `SIGINT`, `chime run` MUST send a
  completion notification that includes `SIGINT` and exit code `130`.
- If the wrapped command exits because of `SIGTERM`, `chime run` MUST send a
  completion notification that includes `SIGTERM` and exit code `143`.
- Signal-triggered exits are still command completions for notification
  purposes. Notification delivery failures remain warnings only.
- After notification is attempted, `chime run` MUST exit with the computed
  signal exit code.

Recommended implementation approach on Linux and macOS:

- Use `os/signal.Notify` for `os.Interrupt` and `syscall.SIGTERM` while the child
  is running so the Chime process does not exit before it can observe the child
  result and notify.
- Forward received termination signals to the immediate child process when it is
  still running.
- Use `exec.ExitError.Sys().(syscall.WaitStatus)` to detect `Signaled()`,
  inspect `Signal()`, and compute `128 + int(signal)`.
- Stop signal notification and restore normal signal handling after the child
  has exited.

Minimum acceptable behavior:

- Signal exit-code detection and notification are covered on Linux/macOS.
- Other platforms are out of scope for this command's signal behavior.

Non-goals:

- Managing process groups for all descendant processes.
- Guaranteeing delivery of notifications if Chime itself receives an
  uncatchable signal such as `SIGKILL`.
- Guaranteeing delivery if the OS terminates Chime before the notification
  request completes.

---

## Error Handling

### Bad Usage

These are local usage errors and exit `1`:

- No wrapped command is provided.
- `--` is provided but no command follows it.
- The mandatory `--` delimiter is omitted.

Error messages SHOULD include the expected form:

```text
usage: chime run [flags] -- <command> [args...]
```

### Command Not Found Or Start Failure

If the child process cannot be started, return a normal Chime error and exit
`1`.

Examples:

- executable not found
- permission denied
- invalid executable format
- working directory no longer exists

No completion notification is sent if the command never starts.

### Child Non-Zero Exit

A non-zero child exit is not a Chime error. It is the wrapped command's result.

The implementation MUST:

- Attempt to send the completion notification.
- Exit with the child's code.
- Avoid printing an extra Chime error for the child failure.

### Notification Failure

If notification delivery fails after the child has started:

- Print a warning to stderr.
- Preserve the child exit code.
- Do not retry.
- Do not fail the command because the alert failed.

If notification setup fails after the child exits because the server URL or API
key is missing or invalid, treat it the same as any other notification failure:
print a warning and preserve the child exit code.

---

## Suggested Package And File Layout

```text
internal/cli/
|-- root.go          # register newRunCmd(cfg)
|-- run.go           # cobra command, config resolution, orchestration
|-- run_test.go      # command parsing and behavior tests

internal/exitcode/
|-- exitcode.go      # add silent/code-only sentinel if accepted
|-- codes.go         # no new fixed code required
```

No new server package changes are required.

No new client package changes are required unless tests need injectable
notification behavior.

---

## Suggested Internal API

The command should be structured so process execution and notification can be
tested without spawning the real requested command in every test.

Suggested types:

```go
type runOptions struct {
    agent   string
    message string
    server  string
    key     string
}

type commandResult struct {
    DisplayCommand string
    ExitCode       int
    Started        bool
    Duration       time.Duration
}
```

Suggested functions:

```go
func newRunCmd(cfg **config.Config) *cobra.Command
func runCommand(cmd *cobra.Command, cfgPtr **config.Config, opts *runOptions, argv []string) error
func executeWrappedCommand(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) (commandResult, error)
func notifyCommandComplete(ctx context.Context, cfg *config.Config, opts *runOptions, result commandResult) error
func formatRunMessage(label string, result commandResult) string
```

Implementation may choose a different shape, but tests should not require
network access or long-running external dependencies.

---

## Cobra Wiring

`newRunCmd` MUST be registered in `root.go`'s `registerSubcommands`, consistent
with repository convention:

```go
rootCmd.AddCommand(
    newStartCmd(cfg),
    newStopCmd(cfg),
    newStatusCmd(cfg),
    newNotifyCmd(cfg),
    newRunCmd(cfg),
    newInstallCmd(cfg),
    newConfigCmd(cfg),
)
```

Do not use per-file `init()` self-registration.

Recommended Cobra settings:

```go
cmd := &cobra.Command{
    Use:   "run [flags] -- <command> [args...]",
    Short: "Run a command and notify when it completes",
    Args:  cobra.ArbitraryArgs,
    RunE: func(cmd *cobra.Command, args []string) error {
        return runCommand(cmd, cfg, opts, args)
    },
}
```

The implementation must account for Cobra flag parsing before `RunE`.

---

## Test Plan

### Argument Parsing

- **RUN-1** `chime run` exits `1` and reports usage guidance.
- **RUN-2** `chime run --` exits `1` and reports usage guidance.
- **RUN-3** `chime run test-command` exits `1` and reports that `--` is
  required.
- **RUN-4** `chime run -- test-command --child-flag` passes
  `["test-command", "--child-flag"]` to the executor.
- **RUN-5** `chime run --agent terminal -- test-command` sends agent
  `"terminal"`.
- **RUN-6** `chime run --message "frontend build" -- test-command` includes
  `"frontend build"` in the notification message.

### Command Execution

- **RUN-7** A wrapped command that exits `0` causes `chime run` to exit `0`.
- **RUN-8** A wrapped command that exits `7` causes `chime run` to exit `7`.
- **RUN-9** A wrapped command's stdout is written to Chime stdout.
- **RUN-10** A wrapped command's stderr is written to Chime stderr.
- **RUN-11** A wrapped command can read from stdin.
- **RUN-12** A command that cannot be started exits `1` and sends no
  notification.
- **RUN-13** On Linux/macOS, a wrapped command terminated by `SIGINT` causes
  `chime run` to exit `130`.
- **RUN-14** On Linux/macOS, a wrapped command terminated by `SIGTERM` causes
  `chime run` to exit `143`.

### Notification Payload

- **RUN-15** Successful command completion sends event `"complete"`.
- **RUN-16** Failed command completion still sends event `"complete"`.
- **RUN-17** Signal-terminated command completion still sends event
  `"complete"` on Linux/macOS.
- **RUN-18** Successful command message includes command display, success text,
  and duration.
- **RUN-19** Failed command message includes command display, exit code, and
  duration.
- **RUN-20** Signal-terminated command message includes the signal name,
  computed exit code, and duration on Linux/macOS.
- **RUN-21** Notification request uses the same `--server` precedence as
  `chime notify`.
- **RUN-22** Notification request uses the same `--key` precedence as
  `chime notify`.

### Notification Failure

- **RUN-23** If notification fails after a child exits `0`, the command still
  exits `0` and prints a warning to stderr.
- **RUN-24** If notification fails after a child exits `9`, the command still
  exits `9` and prints a warning to stderr.
- **RUN-25** If notification fails after a child is terminated by `SIGINT` on
  Linux/macOS, the command still exits `130` and prints a warning to stderr.

### Exit-Code Plumbing

- **RUN-26** Child exit code propagation does not print an extra Chime error
  message.
- **RUN-27** The implementation can return arbitrary child exit codes beyond the
  fixed Chime exit-code table.

---

## Documentation Updates

Implementation should update:

- `docs/design.md` CLI Interface section with a short `chime run` entry.
- `docs/specs/cli.md` with the user-facing command contract and testable
  conditions.
- README or install snippets only if the project has user-facing command
  examples there at implementation time.

