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
- `E2E-001` — Real host-and-container smoke harness now binds hardened mode to non-local TCP by default (`BROKER_BIND_HOST=0.0.0.0`), emits actionable readiness diagnostics, and writes machine-readable report output.
- `E2E-002` — Deny-path real-flow verification now passes in the same smoke harness, including explicit audit assertion for `operator_denied_request` with request id and reason.
- `DX-001` — Hygiene portability remediation is now guarded by `scripts/validate_hygiene_portability.sh` and executed by `make hygiene`/`make ci`, preventing reintroduction of GNU-only `find -regextype`.
- `DOC-001` — Added canonical command/endpoint/token matrix at `docs/operations/CLI-ENDPOINT-CONTRACT-MATRIX.md`.
- `DOC-002` — Standardized remediation guidance in runbooks and improved CLI error propagation so HTTP response bodies (for example `request_id required`, `secret backend unavailable`) are preserved in user-facing errors with test coverage.
- `BETA-001` — Expanded MCP conformance coverage for target-client profiles (string IDs, null params, and `id:null` error-envelope checks for parse/batch errors) and updated compatibility docs.
- `REL-001` — Release packaging now uses pinned GoReleaser builds (`.goreleaser.yaml` + `scripts/release-package.sh`) while keeping `make release-package VERSION=...` as the canonical workflow.
- Production-readiness P0/P1/P2 scope remains complete in `docs/plans/initiatives/PRODUCTION-READINESS.md`.
- Go-first tooling migration remains complete in `docs/plans/initiatives/GO-FIRST-TOOLING-MIGRATION.md`.
