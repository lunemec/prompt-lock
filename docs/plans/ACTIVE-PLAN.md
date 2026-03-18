# ACTIVE PLAN

Updated: 2026-03-18

This is the canonical run-to-run handoff file for agents.
Read it with `docs/plans/BACKLOG.md` before implementation work.

## Current focus
- Keep `make validate-final`, `make ci-redteam-full`, and `make real-e2e-smoke` passing.
- Keep the supported hardened dual-socket path stable in release, readiness, red-team automation, and the real host-plus-container wrapper flow across desktop Docker runtimes.
- Keep plan state centralized in `ACTIVE-PLAN.md`, `BACKLOG.md`, and active initiative/checklist docs only.

## Next focus
- Prepare the first public pre-1.0 tag around the local-only hardened dual-socket deployment story.
- Preserve exact secret-byte semantics and durable state consistency when adding new adapters or retry behavior.
- Continue backlog-driven work only; open new gaps in `docs/plans/BACKLOG.md`.

## Open review items
- None currently.
- Historical strict-review remediation is archived in `docs/plans/archive/2026/STRICT-REVIEW-REMEDIATION-2026-03-15.md`.

## Active references
- `docs/plans/BACKLOG.md`
- `docs/plans/checklists/BETA-READINESS.md`
- `docs/plans/status/PRODUCTION-READINESS-STATUS.json`
- `docs/decisions/INDEX.md`

## Delivery gates
- tests first (Red)
- implementation pass (Green)
- security refactor pass (Blue)
- explicit security findings section in task completion
- `make docs` for documentation structure changes
- `make validate-final` before merge
