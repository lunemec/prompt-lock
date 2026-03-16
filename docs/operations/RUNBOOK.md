# RUNBOOK

## Local dev
- Start mock broker (mock-only localhost-TCP demo, not the supported hardened path): `go run ./cmd/promptlock-mock-broker`
- Start real broker in dev profile (explicit opt-in): `PROMPTLOCK_ALLOW_DEV_PROFILE=1 go run ./cmd/promptlockd`
- Request lease: `go run ./cmd/promptlock exec ...`
- Approve lease interactively: `go run ./cmd/promptlock watch`
- Auth lifecycle helpers: `go run ./cmd/promptlock auth <bootstrap|pair|mint> ...`
- CLI/endpoint auth contract matrix: `docs/operations/CLI-ENDPOINT-CONTRACT-MATRIX.md`
- Full host+container lab walkthrough: `docs/operations/REAL-E2E-HOST-CONTAINER.md`

## Security operations
- Keep audit trail on host storage (not container-writable paths).
- Keep operator socket host-only and mount only the agent socket into containers.
- Treat the supported OSS v1 deployment shape as local-only hardened operation with dual Unix sockets.
- For non-dev profiles, ensure `auth.store_file` + auth-store encryption key env are configured, and configure request/lease durability via either `state_store_file` (`state_store.type=file`) or external state adapter settings (`state_store.type=external`).
- Treat `state_store.type=external` as a durability/availability integration, not a concurrency-safe multi-node coordinator.
- For SOPS-managed runtime keys, set `PROMPTLOCK_SOPS_ENV_FILE=/path/to/runtime-keys.sops.env` before starting `promptlockd`; startup fails closed when decryption or required key loading fails.

### Storage fsync preflight (before production rollout)
- Run this once per target storage mount (as the same service user PromptLock runs under):

```bash
make storage-fsync-preflight MOUNT_DIR=/var/lib/promptlock
```

- Direct command form:

```bash
go run ./cmd/promptlock-storage-fsync-check --dir /var/lib/promptlock
```
- Before generating or validating JSON reports, export signing key material:

```bash
export PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY='<32+ character HMAC key>'
export PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID='release-key-2026-03'
# optional rotation overlap verification:
export PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV='<previous 32+ character HMAC key>'
export PROMPTLOCK_STORAGE_FSYNC_HMAC_KEYRING='release-key-2026-02:PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV'
export PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE='168h'
```
- SOPS alternative for fsync key material:

```bash
export PROMPTLOCK_SOPS_ENV_FILE=/etc/promptlock/fsync-keys.sops.env
```

- Decrypted SOPS payload must provide the same env names used by fsync tooling (`PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY`, `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID`, and optional keyring env vars).
- Multi-mount JSON evidence report:

```bash
make storage-fsync-report MOUNT_DIRS=/var/lib/promptlock,/var/log/promptlock FSYNC_REPORT=reports/storage-fsync-report.json
# optional explicit file path override:
# make storage-fsync-report MOUNT_DIRS=/var/lib/promptlock,/var/log/promptlock FSYNC_REPORT=reports/storage-fsync-report.json SOPS_ENV_FILE=/etc/promptlock/fsync-keys.sops.env
```
- Report provenance fields include metadata (`schema_version`, `generated_at`, `generated_by`, `hostname`) plus attestation fields (`signature.alg`, `signature.key_id`, `signature.value`).
- Validate a pre-generated report (fails if JSON is malformed, signature is missing/invalid, or any mount has `ok=false`):

```bash
make storage-fsync-validate FSYNC_REPORT=reports/storage-fsync-report.json
# optional explicit file path override:
# make storage-fsync-validate FSYNC_REPORT=reports/storage-fsync-report.json SOPS_ENV_FILE=/etc/promptlock/fsync-keys.sops.env
```
- Rotation fail-closed behavior for validator:
  - `signature.key_id` must match either active key id (`PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID`) or a key-id present in `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEYRING`.
  - Non-primary key ids are accepted only when report `generated_at` is within `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE`.
  - Unknown key ids, malformed keyring entries, missing referenced key env vars, disabled overlap windows, and expired overlaps all fail validation.
- One-shot release/readiness gate (generate + validate report in one command):

```bash
make storage-fsync-release-gate MOUNT_DIRS=/var/lib/promptlock,/var/log/promptlock FSYNC_REPORT=reports/storage-fsync-report.json
# optional explicit file path override:
# make storage-fsync-release-gate MOUNT_DIRS=/var/lib/promptlock,/var/log/promptlock FSYNC_REPORT=reports/storage-fsync-report.json SOPS_ENV_FILE=/etc/promptlock/fsync-keys.sops.env
```
- If this check fails, do not deploy PromptLock persistence on that mount; durability-gate behavior will fail closed (`503` on mutating auth/lease endpoints), including when report signatures are missing or invalid.

### Audit integrity verification
- Verify full hash-chain:
  - `go run ./cmd/promptlock audit-verify --file /var/log/promptlock/audit.jsonl`
- Optional local checkpoint continuity check:
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
   - `go run ./cmd/promptlock-readiness-check --file docs/plans/status/PRODUCTION-READINESS-STATUS.json --require-release-gating`
   - `--require-release-gating` fails on explicit release-gating rows only: tasks with `priority:"P0"` or `blocking:true`. A `P0-*` id alone does not count, and blank-identity release-gating rows are rejected.
2. Run baseline CI gate:
   - `make validate-final`
3. Run hardened smoke suite:
   - `make hardened-smoke`
4. Confirm live red-team hardened report exists:
   - `cat reports/redteam-live-hardened.json`
5. Verify audit chain immediately after startup:
   - `go run ./cmd/promptlock audit-verify --file /var/log/promptlock/audit.jsonl`

## Incident quick-reference
- **Auth/session anomalies**
  - Inspect recent audit events for `auth_*` and `secret_backend_error`.
  - If using persisted auth store, verify `auth.store_file` integrity/permissions.
- **Secret backend failures**
  - Validate `secret_source.type` and provider-specific settings (`env_prefix` / `file_path`).
  - Expect deterministic client error: `secret backend unavailable`.
- **Durability gate closed (`503` on auth/lease mutations)**
  - Inspect audit for `durability_persist_failed` / `durability_gate_closed` and check host disk/permissions for `auth.store_file` plus `state_store_file` when using local file state.
  - For `state_store.type=external`, verify backend reachability/auth token env and backend status; state backend outages now fail closed with `503`.
  - Confirm the underlying filesystem supports file + parent-directory `fsync` semantics for local file persistence paths; mounts that reject directory sync will keep the durability gate closed by design.
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
