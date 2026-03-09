# E2E Usability and Operations Gaps

Scope reviewed: real host daemon + untrusted container workflow, operator approval UX, onboarding docs/commands, and failure handling.

Status: Completed (2026-03-09).

Canonical task status is tracked in `docs/plans/BACKLOG.md`.

---

## Completion summary

All previously open items from this initiative are now complete in the current worktree:

1. Real host+container smoke harness transport mismatch was fixed.
2. Deny-path smoke verification now passes and validates audit evidence.
3. Hygiene portability now has a regression guard in CI (`make hygiene`).
4. A canonical CLI/endpoint/token contract matrix now exists.
5. Operator-facing error remediation now has explicit docs plus test-backed CLI error propagation.

---

## Task group A — CLI completeness for real auth lifecycle

### A1 — Add `promptlock auth bootstrap` command
- **Status:** ✅ Completed (2026-03-09)

### A2 — Add `promptlock auth pair` command
- **Status:** ✅ Completed (2026-03-09)

### A3 — Add `promptlock auth mint` command
- **Status:** ✅ Completed (2026-03-09)

### A4 — Optional automation path in `promptlock exec`
- **Status:** Deferred (optional; not part of canonical open backlog)
- Notes:
  - This remains intentionally out-of-scope for current readiness closure.
  - Any future implementation should preserve operator approval semantics and full auditability.

---

## Task group B — Single source-of-truth reality runbook

### B1 — Create canonical "Host daemon + container agent" guide
- **Status:** ✅ Completed (2026-03-09)

### B2 — Reconcile README/RUNBOOK/DOCKER examples
- **Status:** ✅ Completed (2026-03-09)

---

## Task group C — Real-flow smoke automation

### C1 — Add E2E host+container smoke harness
- **Status:** ✅ Completed (2026-03-09)
- Verification:
  - `scripts/run_real_e2e_smoke.sh` now:
    - binds hardened startup to configurable non-local TCP (`BROKER_BIND_HOST`, default `0.0.0.0`) to avoid local-address unix-socket auto-selection mismatch,
    - emits actionable startup diagnostics (including transport mismatch hints),
    - writes machine-readable report output under `reports/real-e2e-smoke.json`.
  - Included in optional full profile via `make ci-redteam-full`.
- Strict gates:
  - [x] Produces machine-readable report under `reports/`.
  - [x] Included in optional CI profile (nightly/full).
  - [x] Fails with actionable diagnostics.

### C2 — Add deny-path counterpart
- **Status:** ✅ Completed (2026-03-09)
- Verification:
  - Harness deny path verifies deterministic CLI denial (`request denied`).
  - Harness asserts `operator_denied_request` audit event contains both `request_id` and deny `reason`.
- Strict gates:
  - [x] Request denied by operator returns deterministic error to agent CLI.
  - [x] Audit events include deny reason and request id.

---

## Task group D — Portability and developer experience

### D1 — Fix BSD/macOS hygiene compatibility
- **Status:** ✅ Completed (2026-03-09)
- Verification:
  - `scripts/validate_repo_hygiene.sh` uses portable `find ... -print | grep -E ...`.
  - New guard `scripts/validate_hygiene_portability.sh` blocks reintroduction of GNU-only `find -regextype`.
  - `make hygiene` now runs both checks.
- Strict gates:
  - [x] `make ci` portability guard present for Linux and BSD/macOS-compatible `find` usage.
  - [x] Hygiene detection behavior unchanged.

### D2 — Keep Go-first policy enforceable
- **Status:** ✅ Completed (2026-03-09)

---

## Task group E — Safety/clarity quality gates

### E1 — Endpoint/CLI contract matrix doc
- **Status:** ✅ Completed (2026-03-09)
- Deliverable:
  - Added `docs/operations/CLI-ENDPOINT-CONTRACT-MATRIX.md`.
- Strict gates:
  - [x] Operator vs agent token requirements are explicit.
  - [x] Used in troubleshooting sections for auth and endpoint errors.

### E2 — Error-message consistency pass (operator-facing)
- **Status:** ✅ Completed (2026-03-09)
- Verification:
  - CLI HTTP helpers now propagate response body text in errors (for example `request_id required`, `secret backend unavailable`) alongside status.
  - Added test coverage for no-session precondition, wrong-endpoint guidance propagation, denied request path, and backend-unavailable propagation.
  - Runbook/real-flow docs now point to a single remediation matrix.
- Strict gates:
  - [x] Common failures include remediation guidance.
  - [x] Integration-oriented CLI tests assert key error strings.

---

## Release readiness impact
- Before completion: secure internals were strong, but real-flow automation, portability guardrails, and operator contract clarity were incomplete.
- After completion: real-flow validation is automated and CI-attachable, portability regressions are guarded, and operator auth/error contracts are explicit and test-backed.
