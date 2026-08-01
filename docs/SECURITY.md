# Scuta Security

## Threat Model

Scuta is a CLI tool manager that downloads, verifies, and installs binaries from GitHub Releases. Its primary trust boundary is between the user's machine and external sources (GitHub API, release assets, registry).

### What Scuta protects against

- **Tampered releases**: SHA256 checksum verification (fail-closed: missing checksums block install unless `--skip-verify`)
- **Forged binaries (opt-in)**: detached `.sig` signature verification against a locally configured trust root (`signature_public_key`, supports RSA, ECDSA, Ed25519). With `require_signature true`, installs fail closed when no signature is found.
- **Releases not built by the expected repo (opt-in)**: cosign keyless signatures and SLSA build provenance verified via the `cosign` / `slsa-verifier` CLIs (`provenance_verify auto|require`). The expected signer identity defaults to the release repo's GitHub Actions workflows and can be pinned with `cosign_identity_regexp` / `cosign_oidc_issuer` (local/system config only, never honored from remote config). Present-but-invalid material is always fatal, in any mode.
- **Post-install tampering**: install-time SHA256 hashes are recorded in state; `scuta doctor --audit` re-hashes every managed binary and reports drift (modified outside scuta).
- **Malicious archives**: Path traversal prevention, symlink rejection, per-file size limits (100 MB)
- **SSRF via config**: URL config values (`github_base_url`, `policy_url`, `registry_url`) require HTTPS and reject loopback/private IPs
- **Tool name injection**: Tool names are validated to prevent path traversal in the bin directory
- **Untrusted download hosts**: Asset download URLs are validated against known GitHub hosts
- **Content-Type confusion**: JSON API responses are validated for correct Content-Type before parsing
- **File permission leaks**: Config, state, and bin directories use restrictive permissions (0600/0700)
- **Concurrent installs**: File-based locking prevents race conditions during parallel installs
- **Metadata tampering (opt-in)**: Remote registry, policy, and org config fetches are verified against detached signatures (`<url>.sig`) using a locally configured trust root (`signature_public_key`). With `require_signed_metadata`, unsigned metadata is rejected outright. The trust root is only honored from local/system config; a remote config can never supply its own key.

### What Scuta does NOT protect against

- **Compromised upstream repos**: If a maintainer's GitHub account is compromised and a legitimate-looking release is published with valid checksums, Scuta will install it. Signature and provenance verification raise the bar (the attacker also needs the signing key, or must produce the release through the repo's own build workflow), but a full repo takeover can still satisfy keyless provenance.
- **Registry poisoning**: A compromised remote registry could redirect tool names to malicious repos, unless signed metadata is enabled (`signature_public_key` + `require_signed_metadata`), which rejects registries not signed by your key. See [REGISTRY.md](./REGISTRY.md#signing-your-registry).
- **Supply chain attacks on Scuta itself**: If the Scuta binary itself is compromised, all bets are off. Users should verify the Scuta binary via Homebrew or checksums.

## Security Roadmap

### Binary Signature Verification

**Status: Implemented**, two layers:

- Key-based: detached `.sig` files verified against `signature_public_key` (RSA, ECDSA, Ed25519), with `require_signature` for fail-closed enforcement. Signed bundles use the same trust root: `bundle create --sign` signs a manifest that pins every asset by SHA-256, and an invalid bundle signature is always fatal.
- Keyless: cosign signatures and SLSA build provenance via `provenance_verify` (see above). Requires the upstream release to ship sigstore bundles, `.sig`+cert pairs, a cosign-signed `checksums.txt`, or `*.intoto.jsonl` attestations.

### Registry Pinning / Integrity

**Status: Implemented** as signed metadata. Operators sign `registry.yaml` (and remote policy/config) with `scuta admin sign`; clients verify against a locally configured public key, failing closed with `require_signed_metadata`. See [REGISTRY.md](./REGISTRY.md#signing-your-registry). For fully offline setups, `registry_url=local` still disables remote fetching entirely.

### Lock File Stale Timeout

Lock files currently expire after 1 hour or when the holding process is no longer running. There is no configurable TTL. In edge cases (process crash without cleanup on a different host), stale locks may persist until the 1-hour timeout.

**Mitigation**: `scuta doctor` detects and reports stale locks. Users can also use `--force` to override a stale lock.

## Recommendations

- Run `scuta doctor` periodically to detect stale locks, missing binaries, and configuration issues; `scuta doctor --audit` additionally checks every managed binary for drift and surfaces recorded provenance
- In high-security environments, enable the fail-closed stack: `require_signature`, `require_signed_metadata`, and `provenance_verify require`
- Use `registry_url=local` in high-security environments to disable remote registry fetching
- Review installed tool sources with `scuta list` (shows whether each tool came from the remote, embedded, or local registry)
- Keep Scuta updated to receive security fixes
