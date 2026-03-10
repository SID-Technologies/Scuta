# Scuta

SID Developer Toolbox. Install once, get everything.

Scuta manages SID's developer CLI tools — installing, updating, and discovering them from a single command.

## Installation

### Homebrew (macOS/Linux)

```bash
brew install sid-technologies/scuta/scuta
```

### Go Install

```bash
go install github.com/sid-technologies/scuta@latest
```

### From Source

```bash
git clone https://github.com/sid-technologies/Scuta.git
cd Scuta
make build
./dist/scuta init
```

## Quick Start

```bash
# Set up Scuta on your machine (interactive setup)
scuta init

# Install all SID tools
scuta install --all

# Check what's available
scuta list

# Update everything
scuta update
```

## Available Tools

| Tool | Description |
|------|-------------|
| `api-gen` | OpenAPI code generator (Go + TypeScript) |
| `pilum` | Multi-cloud deployment CLI |
| `mcp-gen` | Generate MCP servers from OpenAPI specs |

## Commands

### Core

| Command | Description |
|---------|-------------|
| `scuta init` | Setup ~/.scuta/, detect auth, configure PATH |
| `scuta install <tool>` | Install a tool from the registry |
| `scuta install <tool> --from <archive>` | Install from a local archive (offline) |
| `scuta install --all` | Install all tools |
| `scuta uninstall <tool>` | Remove a tool |
| `scuta update [tool]` | Update one or all tools |
| `scuta list` | Show all tools + versions + install status |
| `scuta info <tool>` | Show detailed information about a tool |
| `scuta doctor` | Health check (PATH, binaries, state) |
| `scuta history` | Show install/update history |
| `scuta self-update` | Update Scuta itself |
| `scuta version` | Print version |

### Configuration

| Command | Description |
|---------|-------------|
| `scuta config list` | Show all config values |
| `scuta config get <key>` | Get a config value |
| `scuta config set <key> <value>` | Set a config value |
| `scuta config reset <key>` | Reset a config value to its default |

Valid config keys: `update_interval`, `github_token`, `registry_url`, `github_base_url`, `policy_url`

### Registry

| Command | Description |
|---------|-------------|
| `scuta registry list` | List local registry entries |
| `scuta registry list --all` | Show merged registry with source info |
| `scuta registry add <name> --repo <owner/repo>` | Add a tool to the local registry |
| `scuta registry remove <name>` | Remove a tool from the local registry |

### Shell Completions

```bash
scuta completion bash > /etc/bash_completion.d/scuta
scuta completion zsh > "${fpath[1]}/_scuta"
scuta completion fish > ~/.config/fish/completions/scuta.fish
```

## Registry Modes

During `scuta init`, you choose a registry mode:

| Mode | Description |
|------|-------------|
| **Public** (default) | Uses the official SID registry — no auth needed |
| **Private** | Uses a private GitHub-hosted registry (requires token) |
| **Local only** | No remote registry — manage tools manually via `scuta registry add` |

Change anytime with `scuta config set registry_url <url>` or `scuta config set registry_url local`.

## Global Flags

| Flag | Description |
|------|-------------|
| `--verbose, -v` | Show detailed output |
| `--quiet, -q` | Suppress non-error output |
| `--json` | Output in JSON format |

## License

BSL 1.1
