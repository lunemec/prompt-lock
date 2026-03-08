# Security Review Tasks (2026-03-08)

Source: strict security review requested by Lukas.
Status: Open.

## S-001 — Add OSS license metadata
- **Priority:** P0
- **Task:** Add an explicit `LICENSE` file at repo root.
- **Why:** OSS publication blocker.
- **Done when:**
  - [ ] SPDX-compatible license file exists.
  - [ ] README references the license.
  - [ ] CI/package checks include license presence.

## S-002 — Add vulnerability disclosure policy
- **Priority:** P0
- **Task:** Create `SECURITY.md` with reporting channel, SLA, supported versions, coordinated disclosure policy.
- **Why:** Security posture and OSS trust requirement.
- **Done when:**
  - [ ] `SECURITY.md` present at repo root.
  - [ ] Includes private reporting method + expected response timelines.
  - [ ] Links from README and docs/operations/RELEASE.md.

## S-003 — Harden auth token comparison
- **Priority:** P0
- **Task:** Replace direct string compare for operator token checks with constant-time comparison.
- **Why:** Remove timing side-channel class.
- **Done when:**
  - [ ] `requireOperator` uses `crypto/subtle` constant-time comparison.
  - [ ] Unit tests cover valid/invalid token behavior unchanged.
  - [ ] No regressions in authz matrix tests.

## S-004 — Publish hardened deployment baseline
- **Priority:** P0
- **Task:** Add/expand docs for hardened deployment defaults (unix socket preferred, no insecure TCP override in production).
- **Why:** Prevent unsafe copy-paste deployments.
- **Done when:**
  - [ ] New section in `docs/operations/DOCKER.md` and/or `CONFIG.md`.
  - [ ] Explicit “production baseline” checklist.
  - [ ] Insecure override (`PROMPTLOCK_ALLOW_INSECURE_TCP`) flagged as emergency-only.

## S-005 — Clarify non-production storage limitations
- **Priority:** P0
- **Task:** Document that current auth/secret stores are in-memory and not production-grade without external backends.
- **Why:** Avoid misleading security assumptions.
- **Done when:**
  - [ ] README and architecture docs include explicit warning.
  - [ ] “Production requirements” section lists required external backends.

## S-006 — Add red-team E2E abuse suite
- **Priority:** P1
- **Task:** Create adversarial E2E tests for auth bypass, token replay, policy bypass attempts, and egress bypass attempts.
- **Why:** Validate controls against attacker behavior, not only happy paths.
- **Done when:**
  - [ ] New test plan doc + executable test harness script.
  - [ ] CI target runs these tests (or nightly profile).
  - [ ] Failures emit actionable security findings.

## S-007 — Tighten execution policy defaults in hardened profile
- **Priority:** P1
- **Task:** Review command policy defaults for `leases/execute` and host docker mediation to reduce accidental over-permissioning.
- **Why:** Execution primitives are highest-risk surfaces.
- **Status:** ✅ Completed (2026-03-08)
- **Done when:**
  - [x] Hardened profile default allowlists minimized.
  - [x] New negative tests for shell-wrapper abuse and argument smuggling.
  - [x] Release notes call out behavior changes.

## S-008 — Strengthen audit-chain operational controls
- **Priority:** P2
- **Task:** Make audit verification/anchoring workflow explicit and operator-friendly.
- **Why:** Tamper-evidence needs operational adoption, not only implementation.
- **Done when:**
  - [ ] `docs/operations/RUNBOOK.md` includes periodic verification routine.
  - [ ] Incident playbook includes audit-integrity failure handling.
  - [ ] Example command snippets provided for routine checks.
