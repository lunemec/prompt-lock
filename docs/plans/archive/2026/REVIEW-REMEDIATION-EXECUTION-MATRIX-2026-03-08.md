# Review Remediation Execution Matrix (2026-03-08)

Combines:
- `docs/plans/archive/2026/SECURITY-REVIEW-TASKS-2026-03-08.md`
- `docs/plans/archive/2026/ARCHITECTURE-REVIEW-TASKS-2026-03-08.md`

## Delivery strategy
- **Wave 0 (Launch blockers):** finish all P0 security items first.
- **Wave 1 (Stabilization):** P1 security + architecture in controlled parallel lanes.
- **Wave 2 (Hardening):** P2 architecture/ops polish.

## Ownership model
- **SEC** = security engineer
- **BE** = backend engineer
- **ARCH** = architecture owner
- **DOCS** = docs/release owner
- **QA** = test/release validation

## Wave 0 — MUST before broader OSS launch

| Order | Task ID | Owner | Parallel lane | Depends on | Notes |
|---|---|---|---|---|---|
| 0.1 | S-001 LICENSE | DOCS | L1 | - | hard OSS blocker |
| 0.2 | S-002 SECURITY.md | SEC + DOCS | L1 | - | disclosure policy |
| 0.3 | S-003 constant-time token compare | BE + SEC | L2 | - | small code change + tests |
| 0.4 | S-004 hardened deployment baseline docs | SEC + DOCS | L1 | - | unix-socket-first baseline |
| 0.5 | S-005 in-memory storage warning | DOCS | L1 | - | explicit non-prod boundary |
| 0.6 | Wave-0 validation gate | QA | L3 | 0.1-0.5 | `make ci` + targeted auth tests |

### Wave 0 merge rule
- Prefer **sequential merges** to `main` due security-sensitive docs consistency.
- Allowed parallel work in feature branches, but merge in this order: docs blockers (S-001/S-002) → code hardening (S-003) → deployment docs (S-004/S-005) → validation.

## Wave 1 — SHOULD before v1.0

| Order | Task ID | Owner | Parallel lane | Depends on | Notes |
|---|---|---|---|---|---|
| 1.1 | S-006 red-team E2E abuse suite | SEC + QA | L2 | Wave 0 | new harness/tests |
| 1.2 | S-007 execution policy defaults tighten | SEC + BE | L2 | Wave 0 | may alter behavior |
| 1.3 | A-001 move policy enforcement to app layer | ARCH + BE | L3 | Wave 0 | refactor-heavy |
| 1.4 | A-002 split promptlockd contexts | ARCH + BE | L3 | A-001 | reduce coupling |
| 1.5 | A-003 service interfaces for control-plane | ARCH + BE | L3 | A-001 | improve testability |
| 1.6 | A-004 architecture conformance tests | ARCH + QA | L3 | A-001..A-003 | CI guardrail |

### Wave 1 merge rule
- Use **topic branches** and merge by track:
  1) security behavior changes (S-006/S-007),
  2) architecture refactor (A-001..A-003),
  3) conformance checks (A-004).
- Require green CI + one reviewer from security for security track.

## Wave 2 — NICE post-launch hardening

| Order | Task ID | Owner | Parallel lane | Depends on | Notes |
|---|---|---|---|---|---|
| 2.1 | S-008 audit-chain ops controls | SEC + DOCS | L1 | Wave 1 | runbook quality |
| 2.2 | A-005 architecture map refresh | ARCH + DOCS | L1 | Wave 1 | docs/source-of-truth |
| 2.3 | A-006 ADR execution-surface boundary | ARCH | L1 | Wave 1 | future drift prevention |
| 2.4 | A-007 error taxonomy mapping | BE + ARCH | L2 | A-001/A-002 | contract consistency |

## Delegation recommendation (subagents)
Given current scope, **parallel subagent execution is safe only for Wave 0 docs tasks**.

- Safe parallel candidates now:
  - Agent D1: S-001 + S-005 (LICENSE + warnings)
  - Agent D2: S-002 + S-004 (SECURITY.md + hardened deployment docs)
- Keep S-003 (constant-time compare) in a separate lane and merge after docs.

For Wave 1 refactors (A-001+), avoid broad parallel merges at first due overlap risk in `cmd/promptlockd/*` and app wiring.

## Immediate next sprint (recommended)
1. Complete Wave 0 in one short sprint.
2. Tag release as `beta` with explicit security boundary language.
3. Start Wave 1 with A-001 design RFC before code changes.
