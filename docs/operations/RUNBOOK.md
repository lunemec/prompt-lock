# RUNBOOK

## Local dev
- Start mock broker: `go run ./cmd/promptlock-mock-broker`
- Start real broker in dev profile (explicit opt-in): `PROMPTLOCK_ALLOW_DEV_PROFILE=1 go run ./cmd/promptlockd`
- Request lease: `go run ./cmd/promptlock exec ...`
- Approve lease interactively: `go run ./cmd/promptlock approve-queue`
- Auth lifecycle helpers: `go run ./cmd/promptlock auth <bootstrap|pair|mint> ...`
- CLI/endpoint auth contract matrix: `docs/operations/CLI-ENDPOINT-CONTRACT-MATRIX.md`
- Full host+container lab walkthrough: `docs/operations/REAL-E2E-HOST-CONTAINER.md`
- Hardened TCP mTLS baseline: `docs/operations/MTLS-HARDENED.md`

## Security operations
- Keep audit trail on host storage (not container-writable paths).
- Rotate demo secrets before any non-local use.
- Treat this repository as experimental while production-readiness hardening is completed.
- For non-dev profiles, ensure `state_store_file`, `auth.store_file`, and auth-store encryption key env are configured before startup.

### Storage fsync preflight (before production rollout)
- Run this once per target storage mount (as the same service user PromptLock runs under):

```bash
make storage-fsync-preflight MOUNT_DIR=/var/lib/promptlock
```

- Direct command form:

```bash
go run ./cmd/promptlock-storage-fsync-check --dir /var/lib/promptlock
```
- Multi-mount JSON evidence report:

```bash
make storage-fsync-report MOUNT_DIRS=/var/lib/promptlock,/var/log/promptlock FSYNC_REPORT=reports/storage-fsync-report.json
```
- Report provenance fields now include `schema_version`, `generated_at`, `generated_by`, and `hostname` in addition to `ok/results`.
- Validate a pre-generated report (fails if JSON is malformed or any mount has `ok=false`):

```bash
make storage-fsync-validate FSYNC_REPORT=reports/storage-fsync-report.json
```
- One-shot release/readiness gate (generate + validate report in one command):

```bash
make storage-fsync-release-gate MOUNT_DIRS=/var/lib/promptlock,/var/log/promptlock FSYNC_REPORT=reports/storage-fsync-report.json
```
- If this check fails, do not deploy PromptLock persistence on that mount; durability-gate behavior will fail closed (`503` on mutating auth/lease endpoints).

### Audit integrity verification
- Verify full hash-chain:
  - `go run ./cmd/promptlock audit-verify --file /var/log/promptlock/audit.jsonl`
- Optional checkpoint anchoring:
  - `go run ./cmd/promptlock audit-verify --file /var/log/promptlock/audit.jsonl --checkpoint /var/log/promptlock/audit.checkpoint --write-checkpoint`

### Periodic verification routine (recommended)
- Daily (or before release):
  1. `go run ./cmd/promptlock audit-verify --file /var/log/promptlock/audit.jsonl`
  2. `go run ./cmd/promptlock audit-verify --file /var/log/promptlock/audit.jsonl --checkpoint /var/log/promptlock/audit.checkpoint --write-checkpoint`
  3. Record result in ops log with timestamp + operator id.
- After any broker restart/config change:
  1. Run full verify once.
  2. Compare with last checkpoint window.
  3. If mismatch, trigger incident flow below.

## First 30 minutes checklist (hardened deployment)
1. Validate config parse + profile:
   - `go run ./cmd/promptlock-readiness-check --file docs/plans/status/PRODUCTION-READINESS-STATUS.json --require-p0`
2. Run baseline CI gate:
   - `make validate-final`
3. Run hardened smoke suite:
   - `make hardened-smoke`
4. Confirm live red-team hardened report exists:
   - `cat reports/redteam-live-hardened.json`
5. Verify audit chain immediately after startup:
   - `go run ./cmd/promptlock audit-verify --file /var/log/promptlock/audit.jsonl`

## Incident quick-reference
- **Transport/TLS startup failure**
  - Check cert/key/CA paths and permissions.
  - Re-run: `go test ./cmd/promptlockd -run 'TestValidateTLSConfig|TestMTLSRejectsClientWithoutCertificate'`
- **Auth/session anomalies**
  - Inspect recent audit events for `auth_*` and `secret_backend_error`.
  - If using persisted auth store, verify `auth.store_file` integrity/permissions.
- **Secret backend failures**
  - Validate `secret_source.type` and provider-specific settings (`env_prefix` / `file_path`).
  - Expect deterministic client error: `secret backend unavailable`.
- **Durability gate closed (`503` on auth/lease mutations)**
  - Inspect audit for `durability_persist_failed` / `durability_gate_closed` and check host disk/permissions for `state_store_file` + `auth.store_file`.
  - Confirm the underlying filesystem supports file + parent-directory `fsync` semantics; mounts that reject directory sync will keep the durability gate closed by design.
  - Fix storage issue, then restart broker to reopen mutating flows.
- **Endpoint/auth confusion during CLI use**
  - Use `docs/operations/CLI-ENDPOINT-CONTRACT-MATRIX.md` to confirm endpoint and token type for each command.
- **Rollback quick path**
  - Revert to last known-good config and restart broker.
  - Re-run `make hardened-smoke` + audit verify before reopening traffic.

### Incident response for audit integrity failures
- If verification fails, immediately:
  1. Freeze broker writes (stop service or switch to read-only mode).
  2. Preserve current audit files and host/system logs for forensics.
  3. Compare with last known checkpoint and identify divergence window.
  4. Rotate operator/session credentials and re-pair agents.
  5. Resume only after root-cause review and clean checkpoint re-established.
