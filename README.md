# chime

***Note***: Project is not implemented yet, README serves as design/interface reference. Attempting to follow installation or usage instructions will not work.

---

Get notified when your coding agent finishes a task or needs your input — on your laptop, even when the agent is running on a remote machine.

Chime runs a small HTTP server on your laptop that receives events from agent hook scripts and fires a desktop notification and/or plays a sound.

## How it works

1. Start the server on your laptop: `chime start`
2. Install hook scripts on whatever machine the agent runs on (same machine or remote)
3. Go do something else — chime will alert you when the agent is done or waiting

## Installation

> Binaries and package manager support coming soon. For now, build from source.

```sh
git clone https://github.com/you/chime
cd chime
go build -o chime ./cmd/chime
```

## Quick start

**On your laptop:**

```sh
chime start
# API key: chime_a3f9...c821
# Add to remote machines: export CHIME_KEY=chime_a3f9...c821
```

**On the agent machine** (same machine or remote — just needs `chime` in PATH):

```sh
export CHIME_KEY=chime_a3f9...c821
export CHIME_SERVER=http://192.168.1.10:7777  # your laptop's LAN IP

chime install claude-code  # prints hook config to paste into your agent
```

Paste the printed snippet into your agent's hook config and you're done.

## Commands

| Command | Description |
|---|---|
| `chime start` | Start the notification server |
| `chime stop` | Stop the server |
| `chime status` | Show server status |
| `chime notify` | Send a notification event (used by hook scripts) |
| `chime install <agent>` | Print hook snippets for `claude-code`, `codex`, or `aider` |
| `chime config show` | Show current config |
| `chime config set <key> <value>` | Set a config value |
| `chime config key` | Print your API key |
| `chime config key rotate` | Generate a new API key |

## Docs

- [`docs/DESIGN.md`](docs/DESIGN.md) — architecture and design decisions
- [`docs/CLI_SPEC.md`](docs/CLI_SPEC.md) — full command specification