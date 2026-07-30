# Running Your Own Registry

Scuta ships with a tiny curated registry (currently just `pilum`). That is by
design: **the registry is meant to be yours.** Point Scuta at your own list of
tools and it becomes an org-wide version manager for whatever CLIs your team
ships and depends on -- private repos, curated public tools, or both.

This document explains how the registry resolves, the two ways to run your own,
and the full manifest schema.

---

## How resolution works

Scuta merges up to three layers. Later layers win on name conflicts:

```
embedded  (baked into the binary at build time)
   |         registry.yaml in this repo -- the fallback of last resort
   v
remote    (fetched over HTTP, cached in ~/.scuta/registry.yaml for 1h)
   |         default: the official SID registry on GitHub
   |         override: `scuta config set registry_url <url>`
   |         disable:  `scuta config set registry_url local`
   v
local     (~/.scuta/local.yaml -- highest priority, per-machine)
             managed with `scuta registry add|remove`
```

- If the remote fetch fails (offline, bad URL), Scuta falls back to the cache,
  then to the embedded registry -- it never hard-fails on a network blip.
- `registry_url = local` skips remote fetching entirely (air-gapped / fully
  self-managed environments).
- Private remote registries are fetched with your GitHub token
  (`scuta config set github_token <token>` or the `SCUTA_GITHUB_TOKEN` env var).

Inspect what you actually have with:

```bash
scuta registry list --all     # merged view with a SOURCE column (embedded/remote/local)
```

---

## Option A: Local registry (no hosting required)

Best for a single machine, quick experiments, or fully manual control.

```bash
# Opt out of the remote registry entirely (optional but recommended for local-only):
scuta config set registry_url local

# Add tools -- written to ~/.scuta/local.yaml:
scuta registry add ripgrep --repo BurntSushi/ripgrep --description "fast grep"
scuta registry add fzf     --repo junegunn/fzf       --description "fuzzy finder"

scuta registry list          # local entries
scuta install ripgrep        # resolves via your local registry
```

Local entries always win over anything from the remote or embedded registries,
so you can also use `local.yaml` to pin or override a single tool without
hosting a whole registry.

---

## Option B: Host your own remote registry (recommended for teams)

This is the org-wide setup: one `registry.yaml` that every machine points at.

1. **Write a `registry.yaml`** (schema below).

2. **Host it anywhere that serves the raw YAML over HTTPS**, for example:
   - a GitHub raw URL (`https://raw.githubusercontent.com/<org>/<repo>/main/registry.yaml`)
   - a private GitHub repo (Scuta sends your token)
   - an S3 object, an internal artifact server, or any static host

3. **Point Scuta at it** on each machine (or bake it into `scuta init` docs):

   ```bash
   scuta config set registry_url https://raw.githubusercontent.com/acme/tools/main/registry.yaml
   # Private repo/registry? also:
   scuta config set github_token <token>
   ```

4. Engineers now get your whole catalog:

   ```bash
   scuta install --all
   scuta list
   ```

The remote registry is cached in `~/.scuta/registry.yaml` for 1 hour, so a brief
outage of your host does not break installs.

> **Tip:** pair a hosted registry with a committed `scuta.lock.yaml` and
> `scuta sync` to converge every machine to exact pinned versions. The registry
> says *what tools exist*; the lockfile says *which versions this project wants*.

---

## Manifest schema

A registry is a single YAML document with a top-level `tools` map. Each key is
the tool name users type (`scuta install <name>`).

```yaml
tools:
  # Minimal entry: GoReleaser-style repos usually need nothing more.
  pilum:
    description: "Multi-cloud deployment CLI"
    repo: sid-technologies/Pilum          # owner/repo (required)
    homebrew: sid-technologies/tap/pilum  # optional: brew ref for `scuta info`

  # Binary name differs from the tool/repo name:
  ripgrep:
    description: "Recursively search directories"
    repo: BurntSushi/ripgrep
    bin: rg                               # the executable inside the archive

  # Dependencies: installed (in order) before this tool.
  my-app:
    description: "Internal app that shells out to fzf"
    repo: acme/my-app
    depends_on: [fzf]

  # Non-standard asset naming: use a template + os/arch maps.
  fzf:
    description: "Command-line fuzzy finder"
    repo: junegunn/fzf
    # Template vars: {{.Version}} {{.OS}} {{.Arch}}
    asset: "fzf-{{.Version}}-{{.OS}}_{{.Arch}}.tar.gz"
    version_prefix: "none"                # tags are "0.55.0", not "v0.55.0"

  # Rust-style target triples via os/arch remapping:
  bat:
    description: "cat clone with syntax highlighting"
    repo: sharkdp/bat
    asset: "bat-{{.Version}}-{{.Arch}}-{{.OS}}.tar.gz"
    os_map:
      darwin: apple-darwin
      linux: unknown-linux-gnu
    arch_map:
      amd64: x86_64
      arm64: aarch64
```

### Field reference

| Field            | Type              | Required | Description |
|------------------|-------------------|----------|-------------|
| `repo`           | `owner/repo`      | yes      | GitHub repository to fetch releases from. |
| `description`    | string            | no       | Shown in `scuta list` / `scuta info`. |
| `bin`            | string            | no       | Executable name if it differs from the tool name (e.g. `rg` for `ripgrep`). |
| `homebrew`       | string            | no       | Homebrew tap reference, shown in `scuta info`. |
| `depends_on`     | list of strings   | no       | Tools installed (in dependency order) before this one. |
| `asset`          | template string   | no       | Asset filename template when auto-detection is not enough. Vars: `{{.Version}}`, `{{.OS}}`, `{{.Arch}}`. |
| `os_map`         | map string→string | no       | Remap Go OS names (`darwin`, `linux`, `windows`) to the release's naming. |
| `arch_map`       | map string→string | no       | Remap Go arch names (`amd64`, `arm64`) to the release's naming. |
| `version_prefix` | string            | no       | Tag prefix. Defaults to `v`. Set to `none` for unprefixed tags. |

**When do you need `asset`/`os_map`/`arch_map`?** Only when a release does not
follow a conventional `name-os-arch` layout. Scuta first tries heuristic asset
matching (the same logic that powers `scuta install owner/repo` for tools not in
any registry). Templates are the escape hatch for unusual naming like Rust
target triples.

---

## Security notes

- **Checksums:** for registry tools, Scuta verifies SHA256 against the release's
  `checksums.txt` and fails closed if it is missing (override with
  `--skip-verify`). Tools installed by bare `owner/repo` outside a registry are
  best-effort (a missing checksums file warns instead of failing).
- **Signatures (opt-in):** enable org-wide with
  `scuta config set require_signature true` and distribute a PEM public key via
  `scuta config set signature_public_key <pem>`. Installs then require a valid
  `.sig` alongside each asset. Supports RSA, ECDSA, and Ed25519.
- **Policy (opt-in):** enforce allowed/blocked versions and a minimum Scuta
  version from a remote `policy_url`.
- **Signed metadata (opt-in):** clients can verify the registry itself — see
  below.

See [SECURITY.md](./SECURITY.md) for the full model.

---

## Signing your registry

Checksums protect tool downloads, but the registry file itself tells clients
*what* to download. Signing it (and any remote policy or org config) protects
against a compromised or spoofed host.

**1. Operator: generate a key pair (once):**

```bash
scuta admin keygen --out scuta-signing
# scuta-signing.key  — private key, keep offline (0600)
# scuta-signing.pub  — public key, distribute to clients
```

**2. Operator: sign the registry on every change:**

```bash
scuta admin sign registry.yaml --key scuta-signing.key
# writes registry.yaml.sig — publish it next to registry.yaml
```

The same applies to a remote policy (`policy.yaml` → `policy.yaml.sig`) and a
remote org config (`config.yaml` → `config.yaml.sig`). Clients always fetch
the signature from `<url>.sig`.

**3. Clients: trust the key and (optionally) fail closed:**

```bash
scuta config set -- signature_public_key "$(cat scuta-signing.pub)"
scuta config set require_signed_metadata true
```

Or do all of it in one shot on a fresh machine — `init --from` installs the
key as the trust root *before* the first fetch, enables
`require_signed_metadata`, verifies the org config, and saves its URL as
`config_url`:

```bash
scuta init --from https://example.com/scuta/config.yaml --key scuta-signing.pub
```

The [registry-starter](https://github.com/sid-technologies/registry-starter)
template repo has a ready-made layout (registry + org config + policy +
signing CI) for hosting all of this.

Semantics:

| State | Result |
|-------|--------|
| Key set, valid signature | Verified, used |
| Key set, **invalid** signature | Rejected — always, even without `require_signed_metadata` |
| Key set, signature missing, require off | Warning, used unverified |
| Key set, signature missing, require **on** | Rejected (fail closed) |
| `require_signed_metadata` on, no key configured | Rejected — set the key first |

Trust-root rules:

- `signature_public_key` and `require_signed_metadata` are only honored from
  **local and system** config — a remotely fetched org config can never supply
  or replace the trust root.
- A remote org config may *strengthen* settings (flip require flags on) but
  never weaken them.
- Verify any file manually with `scuta admin verify <file> [--pubkey <path>]`.

Signing keys: `scuta admin keygen` produces Ed25519 keys; RSA and ECDSA keys
in PKCS#8/PKIX PEM form work too (same as asset signature verification).
