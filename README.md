# Scuta

SID Developer Toolbox. Install once, get everything.

Scuta manages SID's developer CLI tools — installing, updating, and discovering them from a single command.

> **Bring your own tools.** The bundled registry is intentionally tiny — Scuta is
> built to point at *your* catalog, whether that's a per-machine local registry
> or a self-hosted one shared across your org. See
> [**Run Your Own Registry**](#run-your-own-registry) below, or the full
> [**docs/REGISTRY.md**](docs/REGISTRY.md) for the local vs. hosted setup and
> manifest schema.

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
| `pilum` | Multi-cloud deployment CLI |

## Commands

### Core

| Command | Description |
|---------|-------------|
| `scuta init` | Setup ~/.scuta/, detect auth, configure PATH |
| `scuta init --from <url> [--key <pub.pem>]` | Non-interactive bootstrap from an org config URL |
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
| `scuta rollback <tool>` | Reinstall the previous version from history |
| `scuta sync` | Reconcile installed tools to a declarative manifest (scuta.lock.yaml) |
| `scuta sync --dry-run` | Show the reconciliation plan without applying it |
| `scuta sync --prune` | Also remove installed tools absent from the manifest |
| `scuta sync --check` | Exit 8 if the machine has drifted from the manifest (CI gate) |
| `scuta cache info` | Show download cache location, entry count, and size |
| `scuta cache clear` | Remove all cached downloads |
| `scuta self-update` | Update Scuta itself |
| `scuta version` | Print version |

### Declarative Sync

Pin your whole toolset in a manifest and converge every machine to it. Commit
`scuta.lock.yaml` to a repo, and every engineer runs `scuta sync`:

```yaml
# scuta.lock.yaml
tools:
  # Registry tool, pinned version (shorthand form):
  pilum: "0.7.5"
  # Arbitrary public repo whose binary name differs from the repo name:
  ripgrep:
    version: "14.1.0"
    repo: "BurntSushi/ripgrep"
    bin: "rg"
  # Unpinned — installed only when missing, kept current by `scuta update`:
  bat: "latest"
```

```bash
scuta sync                 # converge to the manifest
scuta sync --dry-run       # preview the plan
scuta sync --prune         # also uninstall tools not listed
scuta sync -f path/to.yaml # use a specific manifest
scuta sync --check         # CI gate: exit 8 on drift, change nothing
```

Sync looks for `scuta.lock.yaml`, `scuta.lock.yml`, `scuta.yaml`, or
`scuta.yml` in the current directory when `-f` is omitted. Pinned versions are
fail-closed on checksum verification for registry-blessed tools; direct repos
fall back to best-effort when a release ships no checksums file.

### Bundles (Offline / Air-gapped)

| Command | Description |
|---------|-------------|
| `scuta bundle create [tool...]` | Create an offline bundle with tool archives |
| `scuta bundle create --from-manifest <file>` | Bundle exactly what a `scuta.lock.yaml` pins |
| `scuta bundle create --platforms <os/arch,...>` | Include builds for multiple target platforms |
| `scuta bundle create --sign <key.pem>` | Embed a signed manifest in the bundle |
| `scuta bundle verify <bundle> [--key <pub.pem>]` | Verify bundle signature and asset checksums |
| `scuta bundle install <bundle>` | Install tools from an offline bundle |

### Configuration

| Command | Description |
|---------|-------------|
| `scuta config list` | Show all config values (merged: system + remote + local) |
| `scuta config get <key>` | Get a config value (effective merged value) |
| `scuta config set <key> <value>` | Set a config value (local config only) |
| `scuta config reset <key>` | Reset a config value to its default |

Valid config keys: `update_interval`, `github_token`, `registry_url`, `github_base_url`, `policy_url`, `config_url`, `telemetry`, `require_signature`, `require_signed_metadata`, `signature_public_key`, `audit_log_destination`, `disable_download_cache`

### Registry

| Command | Description |
|---------|-------------|
| `scuta registry list` | List local registry entries |
| `scuta registry list --all` | Show merged registry with source info |
| `scuta registry add <name> --repo <owner/repo>` | Add a tool to the local registry |
| `scuta registry remove <name>` | Remove a tool from the local registry |

### Admin (registry operators)

| Command | Description |
|---------|-------------|
| `scuta admin keygen` | Generate an Ed25519 signing key pair |
| `scuta admin sign <file> --key <key>` | Create a detached signature (`<file>.sig`) |
| `scuta admin verify <file>` | Verify a file against its detached signature |

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
scuta bundle create pilum

# Bundle exactly what your manifest pins, for multiple platforms, signed:
scuta bundle create --from-manifest scuta.lock.yaml    --platforms darwin/arm64,linux/amd64 --sign scuta-signing.key

# Transfer the .tar.gz bundle to the air-gapped machine, then:
scuta bundle verify ./scuta-bundle-20260319.tar.gz --key scuta-signing.pub
scuta bundle install ./scuta-bundle-20260319.tar.gz

# Or install a single tool from a local archive:
scuta install pilum --from ./pilum_2.1.5_darwin_arm64.tar.gz
```

## Security

Scuta verifies every download:

- **Checksum verification** (default): SHA256 checksums are verified against the release's `checksums.txt`. Fails if checksums are missing (use `--skip-verify` to override).
- **Signature verification** (opt-in): Enable with `scuta config set require_signature true` and provide a PEM public key via `scuta config set signature_public_key <pem>`. Supports RSA, ECDSA, and Ed25519. When enabled, installs fail if no `.sig` file is found.
- **Signed metadata** (opt-in): remote registry, policy, and org config fetches are verified against a detached `.sig` file using the same `signature_public_key` trust root. Enable fail-closed mode with `scuta config set require_signed_metadata true`. Operators sign with `scuta admin keygen` / `scuta admin sign` — see [docs/REGISTRY.md](docs/REGISTRY.md#signing-your-registry).
- **Signed bundles**: `scuta bundle create --sign <key>` signs the bundle manifest (which pins every asset by SHA-256, across all platforms in the bundle). `bundle verify` and `bundle install` check the signature against the `signature_public_key` trust root; an invalid signature is always fatal, and `require_signature true` makes unsigned bundles fail closed (and blocks `--skip-verify` — the policy is authoritative). Asset checksums are verified on every install either way.
- **Policy enforcement**: Organizations can enforce version constraints via a remote `policy_url` — allowed/blocked versions, minimum Scuta version.
- **Download cache**: verified assets are cached content-addressed by SHA-256 under `~/.scuta/cache`, so repeat installs skip the network without weakening verification — only checksum-verified assets are ever cached, and entries are re-hashed on every hit. Inspect with `scuta cache info`; disable with `scuta config set disable_download_cache true`.

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

## Run Your Own Registry

The bundled registry is intentionally tiny — **it is meant to be replaced with
yours.** Point Scuta at your own list of tools and it becomes an org-wide version
manager for whatever CLIs your team ships or depends on (private repos, curated
public tools, or both).

Scuta merges three layers, later wins on conflicts:
**embedded** (baked in) → **remote** (`registry_url`, cached 1h) →
**local** (`~/.scuta/local.yaml`, per-machine).

**Local only — no hosting:**

```bash
scuta config set registry_url local
scuta registry add ripgrep --repo BurntSushi/ripgrep --description "fast grep"
scuta install ripgrep
```

**Host your own — org-wide:** write a `registry.yaml`, serve it over HTTPS
(GitHub raw, a private repo, S3, any static host), and point every machine at it:

```bash
scuta config set registry_url https://raw.githubusercontent.com/acme/tools/main/registry.yaml
scuta config set github_token <token>   # only for private registries/repos
scuta install --all
```

Minimal manifest:

```yaml
# registry.yaml
tools:
  pilum:
    description: "Multi-cloud deployment CLI"
    repo: sid-technologies/Pilum
  ripgrep:
    description: "Recursively search directories"
    repo: BurntSushi/ripgrep
    bin: rg                    # executable name if it differs from the tool name
```

Inspect the merged result anytime:

```bash
scuta registry list --all      # shows a SOURCE column: embedded / remote / local
```

Full schema (asset templates, os/arch maps, version prefixes, dependencies) and
hosting options are in **[docs/REGISTRY.md](docs/REGISTRY.md)**.

During `scuta init` you can pick a mode up front:

| Mode | Description |
|------|-------------|
| **Public** (default) | Uses the official SID registry — no auth needed |
| **Private** | Uses a private GitHub-hosted registry (requires token) |
| **Local only** | No remote registry — manage tools manually via `scuta registry add` |

Change anytime with `scuta config set registry_url <url>` or `scuta config set registry_url local`.

### Org bootstrap

Point a new machine at your org in one command — no prompts, CI-friendly:

```bash
scuta init --from https://example.com/scuta/config.yaml --key org.pub
```

This installs `org.pub` as the local trust root *before* anything is fetched,
enables `require_signed_metadata` (all remote metadata fails closed from then
on), verifies and applies the org config, and saves the URL as `config_url` so
future runs keep pulling org settings (registry, policy, security flags).
Bootstrapping without `--key` works but is unverified — Scuta warns.

To publish an org config, start from the
[registry-starter](https://github.com/sid-technologies/registry-starter)
template.

## Global Flags

| Flag | Description |
|------|-------------|
| `--verbose, -v` | Show detailed output |
| `--quiet, -q` | Suppress non-error output |
| `--json` | Output in JSON format |

## License

BSL 1.1
