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
| `scuta install --system` | Install to system-wide location (requires sudo) |
| `scuta uninstall <tool>` | Remove a tool |
| `scuta uninstall --system` | Uninstall from system-wide location (requires sudo) |
| `scuta update [tool]` | Update one or all tools |
| `scuta update --system` | Update system-wide installations (requires sudo) |
| `scuta list` | Show all tools + versions + install status |
| `scuta info <tool>` | Show detailed information about a tool |
| `scuta doctor` | Health check (PATH, binaries, state, CVEs) |
| `scuta doctor --skip-cve` | Skip CVE check (for offline environments) |
| `scuta history` | Show install/update history |
| `scuta self-update` | Update Scuta itself |
| `scuta version` | Print version |

### Bundles (Offline / Air-gapped)

| Command | Description |
|---------|-------------|
| `scuta bundle create [tool...]` | Create an offline bundle with tool archives |
| `scuta bundle install <bundle>` | Install tools from an offline bundle |

### Configuration

| Command | Description |
|---------|-------------|
| `scuta config list` | Show all config values (merged: system + remote + local) |
| `scuta config get <key>` | Get a config value (effective merged value) |
| `scuta config set <key> <value>` | Set a config value (local config only) |
| `scuta config reset <key>` | Reset a config value to its default |

Valid config keys: `update_interval`, `github_token`, `registry_url`, `github_base_url`, `policy_url`, `config_url`, `telemetry`, `require_signature`, `signature_public_key`, `audit_log_destination`

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

## System-wide Install

Use `--system` to install tools to `/usr/local/bin` (or `C:\Program Files\Scuta\bin` on Windows) instead of `~/.scuta/bin/`. This requires root/admin privileges:

```bash
sudo scuta install --all --system
sudo scuta update --system
sudo scuta uninstall <tool> --system
```

System-wide state is stored in `/etc/scuta/` (or `C:\ProgramData\Scuta` on Windows).

## Offline / Air-gapped

For environments without internet access, use bundles to transport tools:

```bash
# On a connected machine: create a bundle
scuta bundle create pilum api-gen

# Transfer the .tar.gz bundle to the air-gapped machine, then:
scuta bundle install ./scuta-bundle-20260319.tar.gz

# Or install a single tool from a local archive:
scuta install pilum --from ./pilum_2.1.5_darwin_arm64.tar.gz
```

## Security

Scuta verifies every download:

- **Checksum verification** (default): SHA256 checksums are verified against the release's `checksums.txt`. Fails if checksums are missing (use `--skip-verify` to override).
- **Signature verification** (opt-in): Enable with `scuta config set require_signature true` and provide a PEM public key via `scuta config set signature_public_key <pem>`. Supports RSA, ECDSA, and Ed25519. When enabled, installs fail if no `.sig` file is found.
- **Policy enforcement**: Organizations can enforce version constraints via a remote `policy_url` — allowed/blocked versions, minimum Scuta version.

## Telemetry

Telemetry is **opt-in** and **disabled by default**. When enabled, Scuta records events locally to `~/.scuta/telemetry.jsonl`:

- Event type (install, update, uninstall, self-update)
- OS and architecture
- Timestamp

No tool names, versions, or personal information is collected.

```bash
scuta config set telemetry true    # enable
scuta config set telemetry false   # disable
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
