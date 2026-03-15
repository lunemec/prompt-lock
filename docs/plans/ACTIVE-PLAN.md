# ACTIVE PLAN

Updated: 2026-03-15

This is the canonical run-to-run handoff file for agents. Read it together with `docs/plans/BACKLOG.md` before starting implementation work.

## Current focus
- Keep plan state centralized in `ACTIVE-PLAN.md`, `BACKLOG.md`, and initiative/checklist docs.
- Keep `make validate-final`, `make ci-redteam-full`, and `make real-e2e-smoke` passing.
- Open any newly discovered gaps only in `docs/plans/BACKLOG.md`.

## Next focus
- Keep the supported hardened dual-socket validation path stable in release/readiness automation and red-team smoke harnesses.
- Preserve exact secret-byte semantics and state-store consistency across adapters when adding new backends or retry behavior.
- Prepare the first public pre-1.0 tag and release notes around the supported local-only hardened dual-socket deployment story.
- Preserve the current local-only security story and assurance gates while new work stays backlog-driven.
- Any newly discovered gaps should be opened only in `docs/plans/BACKLOG.md`.

## Fresh review notes
- A fresh 2026-03-15 re-review confirmed and fixed `AUTH-008`: auth lifecycle audit events now hash bearer-style `grant_id` / `session_token` references before sink dispatch, so non-file audit sinks, fixtures, and legacy token shapes no longer rely on adapter-side token-pattern sanitization to avoid credential leakage.
- A fresh 2026-03-15 re-review confirmed and fixed `SEC-027`: request/lease mutations plus auth lifecycle and auth cleanup now re-persist the restored snapshot when persistence reports a post-write failure, so post-`rename` parent-directory `fsync` failures no longer leave durable state mutated behind a `503`.
- The same 2026-03-15 re-review confirmed and fixed `AUTH-007`: multi-target auth revoke now restores the full auth snapshot if one target fails before persistence/audit, so a bad `session_token` can no longer leave the requested `grant_id` revoked behind a `404`.
- The same 2026-03-15 re-review confirmed and fixed `SEC-028`: env-path approval/deny now carry `env_path_original` and `env_path_canonical` on the primary `request_approved` / `request_denied` audit records, so those decision paths no longer rely on a second post-commit critical audit write that could force a misleading rollback behind a `503`.
- The same 2026-03-15 re-review confirmed and fixed `SEC-029`: secret backend failure audit now records coarse safe reason classes instead of raw backend error text, so upstream error bodies can no longer leak into host audit logs through `secret_backend_error`.
- The same 2026-03-15 re-review confirmed and fixed `API-006`: request/lease rollback delete operations are now explicit in the state-store port contract and the documented external state API contract, so adapters must implement the delete endpoints required by audit-failure recovery.
- The same 2026-03-15 re-review confirmed and fixed `SEC-030`: secret access now records a critical `secret_access_started` gate before the broker reads secret material from a secret backend or approved env-path source, so audit sink failure can no longer permit an unaudited secret read behind a `503`.
- A fresh 2026-03-14 re-review confirmed and fixed `SEC-024`: file-backed request/approve/deny/cancel flows now commit request/lease state through an app-layer durability hook before emitting the primary success audit event, and they roll back the in-memory mutation if that file-backed commit fails.
- A follow-up 2026-03-14 re-review confirmed and fixed `SEC-025`: if the primary success audit write fails after a file-backed request/lease state commit, the rollback now re-persists the restored snapshot so durable state matches the rolled-back in-memory store.
- The same follow-up 2026-03-14 re-review originally addressed `SEC-026` by making env-path approval/deny depend on a second critical audit write; the 2026-03-15 `SEC-028` follow-up superseded that approach by moving env-path context onto the primary decision audit records and keeping the supplemental env-path events best-effort.

## Recently completed
- Closed `AUTH-008` by hashing bearer-style auth audit metadata at the app layer (`grant_id` / `session_token`) and adding regression coverage that proves auth lifecycle audit events no longer emit raw credential values before they reach the configured sink.
- Closed `SEC-027` by re-persisting rollback snapshots when request/lease state, auth lifecycle state, or auth cleanup persistence returns a post-write failure, and adding regression coverage that proves durable state is restored after simulated post-`rename` failures.
- Closed `AUTH-007` by restoring the auth snapshot when multi-target revoke fails before persistence/audit, and adding regression coverage that proves a missing `session_token` no longer leaves the requested grant partially revoked.
- Closed `SEC-028` by moving env-path approval/deny context onto the primary decision audit records and downgrading the supplemental `env_path_confirmed` / `env_path_rejected` events to best-effort, with regression coverage that proves supplemental env-path audit failure no longer forces a false rollback after the primary audited decision succeeded.
- Closed `SEC-029` by classifying secret backend audit reasons into safe coarse codes and adding regression coverage that proves raw upstream error text no longer lands in `secret_backend_error` audit metadata.
- Closed `API-006` by making `DeleteRequest` / `DeleteLease` part of the request/lease state-store port contract and documenting the matching external state backend `DELETE` endpoints required for rollback recovery.
- Closed `SEC-030` by adding the `secret_access_started` audit gate before secret backend/env-path reads and regression coverage that proves audit failure now blocks secret material reads instead of allowing an unaudited fetch behind a `503`.
- Closed `SEC-026` with an intermediate handler/service hardening pass for env-path approval/deny auditing; that design was superseded by `SEC-028`, which keeps env-path context on the primary decision audit while treating the supplemental env-path events as best-effort.
- Closed `SEC-025` by re-persisting rollback state after post-commit audit failures in the file-backed request/approve/deny/cancel flows, and adding regression coverage that reloads the state file to prove it no longer retains rolled-back mutations.
- Closed `SEC-024` by moving file-backed request/lease durability commits ahead of the primary success audit events, removing the old post-service handler flush, and adding regression coverage that proves persist failures roll back state and avoid false success audit records.
- Closed `SEC-017`, `SEC-022`, and `SEC-023` by making auth lifecycle and auth cleanup rollback on audit failure, adding rollback-safe request/lease mutations across the external state backend, moving operator deny/approve attribution onto the primary service events, and adding pre-dispatch audit gates plus honest post-dispatch warning semantics for broker-exec and host-Docker execution paths.
- Closed `SEC-018`, `SEC-019`, `SEC-020`, `SEC-021`, `AUTH-005`, `AUTH-006`, `API-005`, `MCP-001`, `QA-004`, `QA-005`, `DOC-005`, and `CFG-009` by re-verifying the current tree, fixing the remaining code/docs drift, moving release-readiness to the supported hardened dual-socket smoke path, removing the dead hardened TLS harness path, stabilizing MCP conformance reporting on Unix-socket test paths, correcting auth revoke/operator docs, relabeling the mock-broker workflow, and cleaning canonical config examples to show only enforced settings.
- Fixed newly discovered review regressions by rolling back request state when lease persistence fails mid-approval, preserving exact secret bytes across env/external secret adapters, and keeping the helper/demo scripts compatible with both the real broker and the shipped mock broker without teaching the mock path as the supported hardened flow.
- Closed `AUTH-004`, `ARCH-003`, `QA-003`, and `DOC-004` by aligning pairing-grant docs with the current bearer-style mint semantics, extending `make arch-conformance` with behavior-conformance tests, adding `go vet`, pinned `govulncheck`, targeted race coverage, and PR CI Docker E2E, and removing stale prototype wording/planning drift.
- Closed `SEC-010`, `SEC-011`, `SEC-012`, `CFG-008`, `SEC-013`, `SEC-014`, `SEC-015`, `SEC-016`, `OPS-007`, `DOC-003`, and `API-004` by hardening external secret backends to fail closed in non-dev, honoring configured request policy at runtime, rejecting dangerous secret-derived env names, denying previously unclassified egress target forms, replacing post-buffer truncation with bounded execution capture, fixing audit checkpoint semantics + checkpoint/create durability, narrowing external-state claims to match current semantics, removing silent local socket fallback, documenting the `env_path` trust boundary, and keeping malformed JSON from mutating state.
- Closed `TRANSPORT-001` by removing retained TCP TLS/mTLS broker transport, removing CLI broker TLS flags/envs, deleting the active mTLS operations doc, and aligning README/ops/ADR content to the local-only Unix-socket transport story.
- Closed `CFG-007`, `SEC-009`, and `UX-006` by renaming the canonical execution-policy key to `execution_policy.exact_match_executables` with documented legacy-key precedence, adding broker-managed `execution_policy.command_search_paths` resolution plus PATH-shadowing regression coverage, publishing ADR-0027 for the provenance trust model, and making `promptlockd` handle normal compose teardown without the prior `promptlockd-e2e exited with code 2` noise.
- Closed the remaining strict-review execution gaps by removing ambient env inheritance from broker and local CLI child processes, enforcing exact executable-name allowlist matches, updating docs to describe `redacted` as best-effort only, and strengthening PR CI around `make release-readiness-gate-core`.
- Tightened agent authz boundaries so lease creation uses the authenticated session agent as the canonical identity, and request/lease status/access/execute paths now enforce session ownership with `403` on cross-agent access attempts.
- Narrowed the supported OSS deployment story to local-only hardened dual-socket operation and then removed the retained TCP TLS/mTLS transport path instead of keeping it quarantined.
- Reworked the README into a Docker-first quickstart with one primary copy-paste operator path, a shorter local-dev fallback, and cleaner routing to release/config/security docs for first-time OSS users.
- Re-verified the public Docker/operator flow against the published docs, fixed the manual host/container walkthrough to include the missing agent-image bootstrap and lab execution-policy allowlist, preserved explicit hardened execution-policy overrides needed by that lab path, and removed misleading compose-smoke `$GITHUB_TOKEN` interpolation warnings before re-running the Docker checks.
- Docker-backed `make release-readiness-gate` now passes in this environment after fixing the compose smoke topology (explicit dev-profile opt-in, explicit unauthenticated non-local TCP demo override, Alpine-compatible `sh` command path, and Docker context cache exclusions).
- Normalized env-path canonicalization across request storage and execute-time verification, and aligned env-path tests with platform-specific canonical path forms.
- Hardened `promptlock-validate-security` to ignore repo-local Go cache directories so `make validate-final` remains reproducible after local Go build/test workflows.
- Removed stale Python `bandit` dependency from CI, replaced it with `make leak-guard`, and aligned public docs to a consistent pre-1.0 OSS release-candidate posture.
- Added SOPS-managed key-material loading for runtime/release workflows via shared loader (`internal/sopsenv`), broker startup env preload (`PROMPTLOCK_SOPS_ENV_FILE`), fsync command `--sops-env-file` support, and Make `SOPS_ENV_FILE` wiring with fail-closed required-env enforcement.
- Added MCP `ping` protocol support with conformance + response-schema test coverage and compatibility matrix updates.
- Added MCP lifecycle protocol support for `shutdown` and `exit`, including notification no-response termination behavior and response-schema/conformance coverage.
- Added MCP initialized-lifecycle compatibility handling (`initialized`, `notifications/initialized`) with notification-safe no-op semantics and request-form schema coverage.
- Added MCP single-session lifecycle sequencing coverage (`initialize`, `notifications/initialized`, `tools/list`, `tools/call`) to guard response-order regressions in target client flows.
- Added MCP cancellation-lifecycle compatibility handling (`notifications/cancelled`) with notification-safe no-op semantics and request-form schema/conformance coverage.
- Expanded MCP compatibility with explicit initialize negotiation fields (`protocolVersion`, `capabilities.tools`), added `resources/list` + `prompts/list` empty-list responses for namespace probes, published stricter `execute_with_intent` tool JSON schema constraints, and tightened fail-closed `tools/call` param validation (`-32602` for null/empty/missing-name params; reject non-string command args and non-integer TTL values).
- Tightened MCP capability advertising to `capabilities.tools` only (keep `resources/list` and `prompts/list` as compatibility handlers without over-advertising unsupported namespaces), and added stderr warning coverage for broker cancel-cleanup propagation failures during `notifications/cancelled`.
- Implemented active MCP cancellation semantics for `notifications/cancelled` by request id (`requestId`/`id`), including in-flight request tracking and context cancellation through broker resolve/request/status polling paths so aborted `tools/call` requests fail closed promptly.
- Hardened MCP notification behavior so `tools/call` frames without request `id` are ignored with no broker side effects, with regression coverage to prevent untracked notification-driven execution paths.
- Added agent-session broker cancellation endpoint (`POST /v1/leases/cancel?request_id=...`) with request-ownership enforcement and wired MCP cancellation to call it as best-effort pending-request cleanup when notifications cancel in-flight tool calls.
- Wired tagged release workflow to export optional fsync rotation envs (`PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV`, `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEYRING`, `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE`) so key-overlap validation is directly usable in CI for common rotation paths.
- Added `make release-readiness-gate-core` so non-Docker environments can run release-grade validation/fuzz subsets before final compose smoke on Docker-capable runners.
- Standardized state/secret backend outage handling to fail closed with `503` across request/approve/deny/pending/by-request/access/execute paths, with explicit regression coverage for each endpoint path and external-state happy-path lifecycle coverage.
- Added a tested broker env-override loader (`applyEnvOverrides`) and aligned canonical example/hardened ops docs with `state_store` block guidance for file vs external state backends.
- Added distributed request/lease state backend support via `state_store.type=external` (`internal/adapters/externalstate`) with token-env auth, non-dev HTTPS enforcement, and fail-closed `503` handling for backend unavailability.
- Removed the previously added CLI TLS/mTLS broker transport options so broker-facing commands now support only broker URL and Unix-socket transport selection.
- Added `make release-readiness-gate` and wired tagged release workflow to run it before fsync attestation + packaging.
- Audit file sink now fsyncs each appended record to disk to reduce crash-window audit loss.
- Broker startup now acquires fail-closed single-writer lock files for configured persistence paths to block concurrent state/auth writers.
- Storage fsync validator now supports key rotation verification via keyring env indirection (`PROMPTLOCK_STORAGE_FSYNC_HMAC_KEYRING`) with explicit overlap-window enforcement (`PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE`) so non-primary signatures remain time-bounded and fail closed outside overlap.
- Storage fsync JSON reports are now cryptographically attested with deterministic HMAC payload signing (`signature.alg`, `signature.key_id`, `signature.value`), and validator gates now fail closed when signatures are missing or invalid.
- Storage fsync Make/release workflows now require HMAC key material env (`PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY`, `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID`) so release/readiness evidence cannot pass with metadata-only provenance.
- Storage fsync JSON reports now carry provenance metadata (`schema_version`, `generated_at`, `generated_by`, `hostname`) and validator gates now fail when metadata is missing or malformed.
- Tagged GitHub release workflow now enforces `storage-fsync-release-gate` before packaging and publishes the generated fsync JSON report artifact.
- Added storage fsync report validator command (`cmd/promptlock-storage-fsync-validate`) and Make release/readiness gates (`storage-fsync-validate`, `storage-fsync-release-gate`) that fail closed when any mount result is not `ok=true`.
- Added storage durability preflight command (`cmd/promptlock-storage-fsync-check`) with JSON reporting and Make workflows (`storage-fsync-preflight`, `storage-fsync-report`) for mount-level file+directory `fsync` validation and evidence capture.
- Hardened-profile local-address detection now matches transport safety parsing (hostname/IP aware) so non-IP `127.*` hostnames do not trigger local unix-socket defaults, with regression tests.
- Operations docs now explicitly call out parent-directory `fsync` requirements for persistence durability-gate recovery workflows.
- Transport safety local-address checks now fail closed for non-IP hostnames prefixed with `127.` (for example `127.evil.example`), with regression tests.
- Auth-store and request/lease state atomic persistence paths now fsync parent directories after rename, with fail-closed regression tests.
- Production deployment guardrails were hardened with explicit dev-profile opt-in, encrypted auth-store persistence enforcement for non-dev, and durable request/lease state persistence support.
- Auth-store atomic persistence now uses secure unique temp files to prevent tmp-path symlink clobbering and concurrent tmp collision races.
- Persistence write failures now fail closed via durability gate behavior with explicit audit signaling and `503` responses on mutating auth/lease paths.
- Release packaging verification pass succeeded in this environment via `make release-package VERSION=v0.0.0` (linux amd64 + darwin arm64 cross-build artifacts).
- Release packaging now runs through pinned GoReleaser builds (`.goreleaser.yaml`, default `goreleaser/v2@v2.7.0`) while preserving the `make release-package VERSION=...` interface.
- External HTTP secret-source adapter support (`secret_source.type=external`) was added.
- Production-readiness scope is marked complete in `docs/plans/initiatives/PRODUCTION-READINESS.md`.
- Go-first tooling migration is marked complete in `docs/plans/initiatives/GO-FIRST-TOOLING-MIGRATION.md`.
- E2E usability and operations gap tasks (C1/C2/D1/E1/E2) are now complete in `docs/plans/initiatives/E2E-USABILITY-AND-OPERATIONS-GAPS.md`.
- Beta checklist MCP conformance task is now complete in `docs/plans/checklists/BETA-READINESS.md`.
- Historical review and remediation plans were moved under `docs/plans/archive/2026/`.
- The planning surface now uses a canonical handoff file, canonical backlog, typed subdirectories, and a machine-readable status directory.

## Primary references
- `docs/plans/BACKLOG.md`
- `docs/plans/initiatives/E2E-USABILITY-AND-OPERATIONS-GAPS.md`
- `docs/plans/checklists/BETA-READINESS.md`
- `docs/plans/status/PRODUCTION-READINESS-STATUS.json`
- `docs/decisions/INDEX.md`

## Delivery gates
- tests first (Red)
- implementation pass (Green)
- security refactor pass (Blue)
- explicit security findings section in final report
- `make docs` for documentation structure changes
- `make validate-final` before merge
