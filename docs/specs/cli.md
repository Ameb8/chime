# chime CLI — Command Specification

> Behavioral specification for all chime commands, flags, and arguments.
> Each section defines expected behavior in terms of observable outcomes
> (exit codes, stdout/stderr content, config mutations, side effects)
> suitable for use as integration test conditions.

---

## Conventions

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error (bad flags, missing args, IO failure) |
| 2 | Authentication failure (wrong or missing API key) |
| 3 | Server unreachable (connection refused, timeout) |
| 4 | Server already running (`start` only) |
| 5 | Server not running (`stop`, `status` only) |

### Output streams

- **Informational messages** → stdout
- **Errors and warnings** → stderr
- **All server logs** → log file only (never stdout/stderr during normal operation)

### Notation

- `[flag]` — optional
- `<arg>` — required positional argument
- `MUST` — mandatory behavior; violation is a spec failure
- `SHOULD` — strongly recommended; deviation requires justification
- `MAY` — optional behavior

---

## Global Flags

These flags are accepted by every command.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `~/.config/chime/config.yaml` | Path to config file |
| `--help`, `-h` | bool | false | Print command help |
| `--version`, `-v` | bool | false | Print version string |

### Testable conditions

- **G-1** `chime --version` MUST print a version string matching `chime v<semver>` to stdout and exit 0.
- **G-2** `chime --help` MUST print a usage summary including the list of available subcommands and exit 0.
- **G-3** `chime <any-command> --help` MUST print help for that specific command and exit 0.
- **G-4** `chime --config /path/to/custom.yaml <command>` MUST read configuration from the specified path instead of the default.
- **G-5** If `--config` points to a nonexistent file, chime MUST print an error to stderr and exit 1 (except for `chime start`, which MAY create the file).
- **G-6** Any unrecognized flag MUST cause chime to print an error to stderr mentioning the unknown flag and exit 1.
- **G-7** Any unrecognized subcommand MUST cause chime to print an error to stderr and print the top-level help and exit 1.

---

## `chime start`

Start the notification server.

```
chime start [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--bind` | string | `0.0.0.0:7777` | Address and port to listen on |
| `--foreground` | bool | false | Run in the foreground; do not daemonize |
| `--log` | string | `~/.local/share/chime/chime.log` | Log file path |

### Behavior

On first ever run (no config file, no API key stored):
1. Generate a new random API key.
2. Write the key into the config file (creating it with mode `0600` if absent).
3. Print the key and a one-line setup hint to stdout **before** daemonizing.

On subsequent runs:
1. Read existing config and API key.
2. No key is printed.

Daemonizing (default, `--foreground` not set):
1. Fork the server process into the background.
2. Write the child PID to the PID file (`~/.local/share/chime/chime.pid`).
3. Redirect child stdout/stderr to the log file.
4. Parent process prints a single confirmation line and exits 0.

Foreground mode (`--foreground`):
1. Server runs in the calling process; blocks until interrupted.
2. Logs go to stdout (in addition to log file if `--log` is set).
3. `SIGINT` / `SIGTERM` trigger graceful shutdown; process exits 0.

### Testable conditions

**First run:**
- **START-1** When no config file exists, `chime start` MUST create the config file at the default path.
- **START-2** When no API key exists, `chime start` MUST print the generated key to stdout in the form `API key: chime_<hex>`.
- **START-3** When no API key exists, `chime start` MUST print a hint telling the user to set `CHIME_KEY=<key>` on remote machines.
- **START-4** The generated API key MUST match the pattern `chime_[0-9a-f]{64}`.
- **START-5** The config file created on first run MUST have file permissions `0600`.

**Daemonizing:**
- **START-6** After `chime start` returns (without `--foreground`), the PID file MUST exist at the default path.
- **START-7** The PID written to the PID file MUST correspond to a running process.
- **START-8** `chime start` (without `--foreground`) MUST exit 0 in the parent process.
- **START-9** After `chime start`, `GET /health` on the configured bind address MUST return 200 within 2 seconds.

**Foreground mode:**
- **START-10** `chime start --foreground` MUST block and not return until the process receives `SIGINT` or `SIGTERM`.
- **START-11** `chime start --foreground` MUST print a startup line to stdout indicating the address being listened on.
- **START-12** After receiving `SIGTERM`, `chime start --foreground` MUST exit 0.

**Already running:**
- **START-13** If the PID file exists and the PID is a running process, `chime start` MUST print an error to stderr and exit 4.
- **START-14** If the PID file exists but the PID is not a running process (stale PID file), `chime start` MUST remove the stale PID file and start normally.

**Bind flag:**
- **START-15** `chime start --bind 127.0.0.1:9000` MUST start the server on `127.0.0.1:9000`, confirmed by `GET http://127.0.0.1:9000/health` returning 200.
- **START-16** If the specified bind address is already in use, `chime start` MUST print an error to stderr and exit 1.
- **START-17** `chime start --bind :9000` (no host) MUST bind to all interfaces on port 9000.

**Log flag:**
- **START-18** `chime start --log /tmp/test-chime.log` MUST write server logs to `/tmp/test-chime.log`.
- **START-19** If the log file path's parent directory does not exist, `chime start` MUST print an error to stderr and exit 1.

---

## `chime stop`

Stop the running background server.

```
chime stop
```

### Flags

None.

### Behavior

1. Read the PID file.
2. Send `SIGTERM` to the process.
3. Wait up to 5 seconds for the process to exit.
4. If the process exits within 5 seconds, remove the PID file and print a confirmation to stdout, exit 0.
5. If the process does not exit within 5 seconds, print a warning to stderr and exit 1. Do not remove the PID file.

### Testable conditions

- **STOP-1** When a server is running, `chime stop` MUST exit 0.
- **STOP-2** When a server is running, `chime stop` MUST print a confirmation message to stdout (e.g. `Server stopped.`).
- **STOP-3** After a successful `chime stop`, the PID file MUST no longer exist.
- **STOP-4** After a successful `chime stop`, `GET /health` on the previously bound address MUST fail with a connection error.
- **STOP-5** When no PID file exists, `chime stop` MUST print an error to stderr and exit 5.
- **STOP-6** When the PID file exists but the PID is not a running process, `chime stop` MUST remove the stale PID file, print a message to stderr noting the server was not running, and exit 5.
- **STOP-7** `chime stop` MUST not leave orphaned processes after a successful stop.

---

## `chime status`

Print the current server status.

```
chime status
```

### Flags

None.

### Behavior

Reads the PID file. If the PID corresponds to a running process, also queries `GET /health` on the configured bind address to confirm the server is responsive.

### Testable conditions

**Server running:**
- **STATUS-1** When the server is running, `chime status` MUST exit 0.
- **STATUS-2** When the server is running, stdout MUST include the string `running`.
- **STATUS-3** When the server is running, stdout MUST include the PID.
- **STATUS-4** When the server is running, stdout MUST include the bind address.
- **STATUS-5** When the server is running, stdout MUST include the uptime in a human-readable form (e.g. `3h 14m`).
- **STATUS-6** When the server is running, stdout MUST include the path to the log file.

**Server stopped:**
- **STATUS-7** When no PID file exists, `chime status` MUST exit 5.
- **STATUS-8** When no PID file exists, stdout or stderr MUST include the string `stopped`.
- **STATUS-9** When the PID file exists but the process is not running, `chime status` MUST exit 5 and include a message noting the server is not running (stale PID file).

**Server unresponsive:**
- **STATUS-10** When the PID is running but `/health` does not respond within 2 seconds, `chime status` MUST exit 1 and print a warning to stderr indicating the server process is alive but not responding.

---

## `chime notify`

Send a notification event to the chime server.

```
chime notify --event <event> [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--event` | string | — | **Required.** Event type: `complete` or `waiting` |
| `--agent` | string | `""` | Name of the agent tool (e.g. `claude-code`) |
| `--message` | string | `""` | Optional detail string from the agent |
| `--server` | string | value from config | Server base URL |
| `--key` | string | value from config/env | API key |

### Flag and environment variable precedence (highest to lowest)

1. `--flag` on command line
2. `CHIME_SERVER` / `CHIME_KEY` environment variables
3. `client.server` / `auth.key` in config file

### Behavior

1. Resolve server URL and API key per precedence order above.
2. Send `POST /notify` with the JSON payload and `Authorization: Bearer <key>` header.
3. On HTTP 200, exit 0 silently (no stdout output).
4. On any error, print a short message to stderr and exit with the appropriate code.

Errors MUST be non-fatal to the calling shell script. Hook scripts SHOULD use `chime notify ... || true` to ensure agent hooks do not block on chime failures.

### Testable conditions

**Required flags:**
- **NOTIFY-1** `chime notify` with no `--event` flag MUST print an error to stderr and exit 1.
- **NOTIFY-2** `chime notify --event unknown_event` MUST print an error to stderr and exit 1.
- **NOTIFY-3** `chime notify --event complete` with a valid server and key MUST exit 0 and produce no stdout output.
- **NOTIFY-4** `chime notify --event waiting` with a valid server and key MUST exit 0 and produce no stdout output.

**Payload:**
- **NOTIFY-5** `chime notify --event complete --agent claude-code` MUST send a request body containing `"event": "complete"` and `"agent": "claude-code"`.
- **NOTIFY-6** `chime notify --event complete --message "done"` MUST send a request body containing `"message": "done"`.
- **NOTIFY-7** `chime notify --event complete` (no `--agent`) MUST send a request body where `agent` is either absent or an empty string.

**Auth:**
- **NOTIFY-8** If no API key is resolvable from flags, env, or config, `chime notify` MUST print an error to stderr and exit 2.
- **NOTIFY-9** `chime notify --event complete --key wrong_key` against a running server MUST print an error to stderr and exit 2.
- **NOTIFY-10** `CHIME_KEY=<valid_key> chime notify --event complete` MUST succeed (exit 0) without a `--key` flag.
- **NOTIFY-11** A `--key` flag MUST take precedence over a `CHIME_KEY` env var.
- **NOTIFY-12** A `CHIME_KEY` env var MUST take precedence over the key in config.

**Server targeting:**
- **NOTIFY-13** `chime notify --event complete --server http://localhost:9999` MUST send the request to `localhost:9999` regardless of config.
- **NOTIFY-14** `CHIME_SERVER=http://localhost:9999 chime notify --event complete` MUST send to `localhost:9999`.
- **NOTIFY-15** A `--server` flag MUST take precedence over a `CHIME_SERVER` env var.

**Connection errors:**
- **NOTIFY-16** If the server is unreachable (connection refused), `chime notify` MUST exit 3.
- **NOTIFY-17** If the request times out (default timeout: 5 seconds), `chime notify` MUST exit 3.
- **NOTIFY-18** In all error cases, `chime notify` MUST produce output on stderr, not stdout.

---

## `chime install`

Print hook configuration snippets for a given agent tool.

```
chime install <agent> [flags]
```

### Arguments

| Argument | Values | Description |
|----------|--------|-------------|
| `<agent>` | `claude-code`, `codex`, `aider` | The agent tool to generate snippets for |

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server` | string | value from config | Server URL to embed in snippet |
| `--key` | string | value from config | API key to embed in snippet |

### Behavior

Prints a ready-to-paste configuration snippet to stdout. The snippet includes the full `chime notify` invocations with `--server` and `--key` values embedded so the hook script is self-contained on the remote machine.

Each agent has two relevant events: `complete` and `waiting`. The snippet MUST cover both where the agent tool supports them.

A human-readable header MUST be printed before the snippet explaining where to paste it.

### Testable conditions

**Argument validation:**
- **INSTALL-1** `chime install` with no agent argument MUST print an error to stderr listing valid agent names and exit 1.
- **INSTALL-2** `chime install unknown-agent` MUST print an error to stderr listing valid agent names and exit 1.
- **INSTALL-3** `chime install claude-code` with a valid config MUST exit 0.

**Snippet content — general:**
- **INSTALL-4** The output MUST contain at least one `chime notify --event complete` invocation.
- **INSTALL-5** The output MUST contain at least one `chime notify --event waiting` invocation.
- **INSTALL-6** Each `chime notify` invocation in the output MUST include `--agent <agent>`.
- **INSTALL-7** If a server URL is resolvable, each `chime notify` invocation MUST include `--server <url>`.
- **INSTALL-8** If an API key is resolvable, each `chime notify` invocation MUST include `--key <key>`.
- **INSTALL-9** The output MUST include a human-readable comment indicating where the snippet should be pasted.

**Flag overrides:**
- **INSTALL-10** `chime install claude-code --server http://192.168.1.5:7777` MUST embed `http://192.168.1.5:7777` in the snippet regardless of config.
- **INSTALL-11** `chime install claude-code --key mykey` MUST embed `mykey` in the snippet regardless of config.

**Agent-specific snippet format:**
- **INSTALL-12** `chime install claude-code` output MUST be valid JSON (the hooks object).
- **INSTALL-13** `chime install codex` output MUST be valid YAML.
- **INSTALL-14** `chime install aider` output MUST be a shell script (starts with a shebang line).

**No config:**
- **INSTALL-15** If no server URL is configured, the snippet MUST still be printed with a placeholder (e.g. `YOUR_SERVER_URL`) and a warning MUST be printed to stderr.
- **INSTALL-16** If no API key is configured, the snippet MUST still be printed with a placeholder (e.g. `YOUR_API_KEY`) and a warning MUST be printed to stderr.

---

## `chime config`

View and manage configuration.

```
chime config <subcommand> [args]
```

### Subcommands

| Subcommand | Description |
|---|---|
| `show` | Print the current resolved config as YAML |
| `set <key> <value>` | Set a config value |
| `key` | Print the current API key |
| `key rotate` | Generate a new API key and save it |

---

### `chime config show`

```
chime config show
```

Print the full resolved config (file values merged with defaults) as YAML to stdout.

#### Testable conditions

- **CONF-SHOW-1** `chime config show` MUST exit 0 when a valid config exists.
- **CONF-SHOW-2** Output MUST be valid YAML.
- **CONF-SHOW-3** Output MUST include all top-level config keys (`server`, `auth`, `client`, `notifications`, `log`).
- **CONF-SHOW-4** The `auth.key` value in the output MUST be masked (e.g. `chime_a3f9...c821` showing only the first and last 4 hex characters).
- **CONF-SHOW-5** `chime config show` MUST reflect values from a `--config` override if provided.
- **CONF-SHOW-6** If no config file exists, `chime config show` MUST print the default values and exit 0.

---

### `chime config set`

```
chime config set <key> <value>
```

Set a single config value by its dot-notation key (e.g. `server.bind`, `notifications.sound.enabled`).

#### Testable conditions

- **CONF-SET-1** `chime config set server.bind 127.0.0.1:8888` MUST update `server.bind` in the config file to `127.0.0.1:8888`.
- **CONF-SET-2** After `chime config set`, `chime config show` MUST reflect the updated value.
- **CONF-SET-3** `chime config set` with no arguments MUST print an error to stderr and exit 1.
- **CONF-SET-4** `chime config set` with only one argument (key but no value) MUST print an error to stderr and exit 1.
- **CONF-SET-5** `chime config set notifications.sound.enabled false` MUST write boolean `false` (not the string `"false"`) to the config.
- **CONF-SET-6** `chime config set unknown.key value` MUST print an error to stderr naming the unknown key and exit 1.
- **CONF-SET-7** `chime config set auth.key <value>` MUST be rejected with an error directing the user to use `chime config key rotate` instead; exit 1.

---

### `chime config key`

```
chime config key
```

Print the current API key to stdout.

#### Testable conditions

- **CONF-KEY-1** `chime config key` MUST print the full API key to stdout and exit 0.
- **CONF-KEY-2** The printed key MUST match the value stored in the config file exactly (unmasked).
- **CONF-KEY-3** If no key exists in config, `chime config key` MUST print an error to stderr directing the user to run `chime start` to generate one, and exit 1.
- **CONF-KEY-4** The key MUST be the only content on stdout (no labels or decoration), so it can be used in scripts: `export CHIME_KEY=$(chime config key)`.

---

### `chime config key rotate`

```
chime config key rotate
```

Generate a new API key, save it to the config file, and print the new key.

#### Testable conditions

- **CONF-KEYROT-1** `chime config key rotate` MUST exit 0.
- **CONF-KEYROT-2** `chime config key rotate` MUST print the new key to stdout.
- **CONF-KEYROT-3** The new key MUST differ from the previous key.
- **CONF-KEYROT-4** The new key MUST match the pattern `chime_[0-9a-f]{64}`.
- **CONF-KEYROT-5** After rotation, `chime config key` MUST return the new key.
- **CONF-KEYROT-6** After rotation, `chime notify` using the old key against a running server MUST fail with exit 2.
- **CONF-KEYROT-7** After rotation, `chime notify` using the new key against a running server MUST succeed (exit 0). Note: server must be restarted to pick up the new key.
- **CONF-KEYROT-8** `chime config key rotate` MUST print a warning to stderr if a server is currently running, noting that the server must be restarted for the new key to take effect.

---

## Cross-Cutting Conditions

These conditions apply across multiple or all commands.

### Config file resolution

- **CC-1** If `CHIME_CONFIG` environment variable is set, it MUST be used as the config file path for all commands (lower precedence than `--config` flag).
- **CC-2** `--config` flag MUST take precedence over `CHIME_CONFIG` env var.
- **CC-3** All commands that read config MUST tolerate a config file with only a subset of keys defined, using defaults for missing keys.

### Data directory

- **CC-4** The data directory (`~/.local/share/chime/`) MUST be created automatically by the first command that needs it, without requiring manual setup.
- **CC-5** All files written to the data directory (PID file, log file) MUST be created with mode `0600`.

### Signal handling (server process)

- **CC-6** The server process MUST handle `SIGTERM` with graceful shutdown: finish in-flight requests, then exit 0.
- **CC-7** The server process MUST handle `SIGINT` the same as `SIGTERM`.
- **CC-8** On graceful shutdown, the server MUST remove the PID file before exiting.

### Logging

- **CC-9** Log output MUST never appear on stdout or stderr of the parent CLI process during normal daemon operation.
- **CC-10** Each log line MUST include a timestamp, level, and message.
- **CC-11** At log level `debug`, every incoming HTTP request MUST be logged with method, path, and response status.
- **CC-12** At log level `info` (default), incoming requests MUST NOT be logged individually; only startup, shutdown, and error events are logged.