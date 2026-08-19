# Aggregating a Fleet

Scuta is sender-only: it writes a signed, machine-readable audit report on
each machine and exits. There is no scuta server, no agent, and no upload
built in. You point the tooling you already run at the report file.

## The feed

Schedule the audit (launchd agent on macOS, systemd user timer on Linux):

```bash
scuta monitor install --interval 1h --system
```

Each run rewrites `~/.scuta/last-audit.json` atomically, and only when the
findings actually changed, so collectors never see a half-written file and
unchanged machines produce no churn. A run with critical findings exits
non-zero, which launchd/systemd surface as a failed job.

## Trusting the reports

Generate a machine key once and schedule signed runs:

```bash
scuta admin keygen --out machine-key
scuta monitor install --interval 1h --system --sign-key ./machine-key.key
```

Each report gets a detached Ed25519 signature at `last-audit.json.sig`.
Verify on the receiving side with `scuta admin verify last-audit.json
--sig last-audit.json.sig --pubkey machine-key.pub`, or with any Ed25519
implementation. A posture claim you cannot attribute to a machine is just
a JSON file; the signature is what makes it evidence.

## Shipping with what you already have

The report is one JSON document per machine at a known path. Any log
shipper handles that:

- **Splunk**: a `monitor` input on `~/.scuta/last-audit.json`, or a cron
  job that POSTs to HEC:
  ```bash
  curl -sS https://splunk.example.com:8088/services/collector/event \
    -H "Authorization: Splunk $HEC_TOKEN" \
    -d "{\"sourcetype\": \"scuta:audit\", \"event\": $(cat ~/.scuta/last-audit.json)}"
  ```
- **S3 + Athena**: `aws s3 cp ~/.scuta/last-audit.json s3://bucket/scuta/$(hostname).json`
  on a cron; the report is flat enough to query with a JSON SerDe. The
  `summary` object (`criticals`, `warnings`, `tools`, `system_packages`)
  is the fleet dashboard in four columns.
- **Any webhook**: `curl --data-binary @~/.scuta/last-audit.json` to
  whatever receives JSON in your shop.

For SBOM pipelines, schedule the CycloneDX form instead by running
`scuta doctor --audit --system --sbom cyclonedx --output sbom.json` from
your own job.

## What to alert on

- `summary.criticals > 0`: tampered binary, PATH shadowing, or dpkg
  binary drift. This is the page-worthy set.
- `posture.trust_root_configured == false` across the fleet: machines
  installing unverified metadata.
- A machine whose report stops updating: the timer died, or someone
  stopped it.

## Non-goals

Scuta does not ship an ingestion server, a dashboard, or hosted storage,
and the report never leaves the machine unless your tooling moves it.
