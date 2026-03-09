# ACTIVE PLAN

Updated: 2026-03-09

This is the canonical run-to-run handoff file for agents. Read it together with `docs/plans/BACKLOG.md` before starting implementation work.

## Current focus
- Keep plan state centralized in `ACTIVE-PLAN.md`, `BACKLOG.md`, and initiative/checklist docs.
- Maintain passing status for `make validate-final`, `make ci-redteam-full`, and `make real-e2e-smoke`.
- Keep operator-facing docs and MCP compatibility matrix aligned with tested behavior.

## Next focus
- Release packaging and environment-specific verification passes (for example optional external macOS run confirmation) as needed by maintainers.
- Any newly discovered gaps should be opened only in `docs/plans/BACKLOG.md`.

## Recently completed
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
