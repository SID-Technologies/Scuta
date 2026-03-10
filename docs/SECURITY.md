# Scuta Security

## Threat Model

Scuta is a CLI tool manager that downloads, verifies, and installs binaries from GitHub Releases. Its primary trust boundary is between the user's machine and external sources (GitHub API, release assets, registry).

### What Scuta protects against

- **Tampered releases**: SHA256 checksum verification (fail-closed — missing checksums block install unless `--skip-verify`)
- **Malicious archives**: Path traversal prevention, symlink rejection, per-file size limits (100 MB)
- **SSRF via config**: URL config values (`github_base_url`, `policy_url`, `registry_url`) require HTTPS and reject loopback/private IPs
- **Tool name injection**: Tool names are validated to prevent path traversal in the bin directory
- **Untrusted download hosts**: Asset download URLs are validated against known GitHub hosts
- **Content-Type confusion**: JSON API responses are validated for correct Content-Type before parsing
- **File permission leaks**: Config, state, and bin directories use restrictive permissions (0600/0700)
- **Concurrent installs**: File-based locking prevents race conditions during parallel installs

### What Scuta does NOT protect against

- **Compromised upstream repos**: If a maintainer's GitHub account is compromised and a legitimate-looking release is published with valid checksums, Scuta will install it. Binary signature verification (see roadmap below) would add a layer here.
- **Registry poisoning**: A compromised remote registry could redirect tool names to malicious repos. Registry pinning (see roadmap) would mitigate this.
- **Supply chain attacks on Scuta itself**: If the Scuta binary itself is compromised, all bets are off. Users should verify the Scuta binary via Homebrew or checksums.

## Security Roadmap

### Binary Signature Verification

Currently Scuta verifies downloads via SHA256 checksums. Adding GPG or cosign signature verification would provide an additional layer of assurance that binaries were produced by the expected maintainer, not just that they match a checksum file (which could also be tampered with in a compromised release).

**Status**: Not implemented. Would require tools to adopt signing as part of their release process.

### Registry Pinning / Integrity

The remote registry is fetched over HTTPS from GitHub, but there's no hash-pinning mechanism. A hash-pinned registry would allow detection of unexpected changes to the registry manifest.

**Status**: Not implemented. The embedded registry provides a baseline fallback, and `registry_url=local` disables remote fetching entirely for security-conscious users.

### Lock File Stale Timeout

Lock files currently expire after 1 hour or when the holding process is no longer running. There is no configurable TTL. In edge cases (process crash without cleanup on a different host), stale locks may persist until the 1-hour timeout.

**Mitigation**: `scuta doctor` detects and reports stale locks. Users can also use `--force` to override a stale lock.

## Recommendations

- Run `scuta doctor` periodically to detect stale locks, missing binaries, and configuration issues
- Use `registry_url=local` in high-security environments to disable remote registry fetching
- Review installed tool sources with `scuta list` (shows whether each tool came from the remote, embedded, or local registry)
- Keep Scuta updated to receive security fixes
