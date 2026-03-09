# BACKLOG

Updated: 2026-03-09

This is the canonical list of open work. Initiative docs may hold detail, but status should stay aligned here.

## Open items
- None.

## Completed in current worktree
- `PROD-001` — `security_profile=dev` startup now requires explicit opt-in (`PROMPTLOCK_ALLOW_DEV_PROFILE=1`), reducing accidental insecure deployment.
- `PROD-002` — Added durable request/lease state persistence (`state_store_file`) with atomic file save/load support in the in-memory adapter.
- `PROD-003` — Added encrypted auth-store persistence support with non-dev startup enforcement for `auth.store_file` via `auth.store_encryption_key_env`.
- `PROD-004` — Added external HTTP secret source adapter (`secret_source.type=external`) with token-env and timeout support.
- `PROD-005` — Hardened auth-store persistence to use unique temp files for atomic writes, preventing predictable tmp-path symlink clobbering and concurrent tmp collision races.
- `PROD-006` — Persistence failures now fail closed: auth-store and request/lease state write failures close a durability gate, emit explicit audit events, and return `503 Service Unavailable` for mutating auth/lease endpoints.
- `PROD-007` — Transport safety local-address classification now rejects non-IP hostnames that only start with `127.` (for example `127.evil.example`) so non-local TCP is not misclassified as loopback.
- `PROD-008` — Atomic auth and request/lease persistence now fsync parent directories after rename to harden crash-consistency of directory-entry updates.
- `PROD-009` — Hardened-profile local-address detection now uses hostname/IP loopback parsing (not `127.` prefix matching), preventing non-IP `127.*` hostnames from being treated as local.
- `DOC-003` — Operations docs now document parent-directory `fsync` requirements and durability-gate recovery expectations for persistence storage.
- `OPS-001` — Added storage durability preflight command (`promptlock-storage-fsync-check`) with JSON reporting and Make workflows (`storage-fsync-preflight`, `storage-fsync-report`) for validating mount-level file+directory `fsync` support and capturing rollout evidence.
- `OPS-002` — Added fsync report validator command (`promptlock-storage-fsync-validate`) and release/readiness Make gates (`storage-fsync-validate`, `storage-fsync-release-gate`) that fail when report JSON is malformed or any mount check reports `ok=false`.
- `OPS-003` — Storage fsync reports now include provenance metadata (`schema_version`, `generated_at`, `generated_by`, `hostname`) and validator tooling enforces those fields before release/readiness gates pass.
- `REL-002` — Tagged GitHub release workflow now runs `storage-fsync-release-gate` before packaging and uploads the generated fsync report artifact for release evidence.
- `E2E-001` — Real host-and-container smoke harness now binds hardened mode to non-local TCP by default (`BROKER_BIND_HOST=0.0.0.0`), emits actionable readiness diagnostics, and writes machine-readable report output.
- `E2E-002` — Deny-path real-flow verification now passes in the same smoke harness, including explicit audit assertion for `operator_denied_request` with request id and reason.
- `DX-001` — Hygiene portability remediation is now guarded by `scripts/validate_hygiene_portability.sh` and executed by `make hygiene`/`make ci`, preventing reintroduction of GNU-only `find -regextype`.
- `DOC-001` — Added canonical command/endpoint/token matrix at `docs/operations/CLI-ENDPOINT-CONTRACT-MATRIX.md`.
- `DOC-002` — Standardized remediation guidance in runbooks and improved CLI error propagation so HTTP response bodies (for example `request_id required`, `secret backend unavailable`) are preserved in user-facing errors with test coverage.
- `BETA-001` — Expanded MCP conformance coverage for target-client profiles (string IDs, null params, and `id:null` error-envelope checks for parse/batch errors) and updated compatibility docs.
- `REL-001` — Release packaging now uses pinned GoReleaser builds (`.goreleaser.yaml` + `scripts/release-package.sh`) while keeping `make release-package VERSION=...` as the canonical workflow.
- Production-readiness P0/P1/P2 scope remains complete in `docs/plans/initiatives/PRODUCTION-READINESS.md`.
- Go-first tooling migration remains complete in `docs/plans/initiatives/GO-FIRST-TOOLING-MIGRATION.md`.
