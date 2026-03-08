# Production Readiness — Remaining Tasks (2026-03-08)

Decision: **do not ship yet** until production-readiness gates are met.

Status: Open.

## Priority model
- **P0** = production blockers
- **P1** = high-confidence hardening before release candidate
- **P2** = operational excellence follow-up

---

## P0-01 — Close mTLS phase-2 and align docs/tasks
- **Area:** Security / transport
- **Status:** ✅ Completed (2026-03-08)
- **Problem:** mTLS phase-1 exists, but TODO/docs status is inconsistent and hardening boundaries are unclear.
- **Scope:**
  - Define explicit mTLS phase-2 acceptance (cipher/TLS minimums, cert reload behavior, profile interactions).
  - Update all plan files (`OSS-PUBLISH-TODO`, strict plan, ops docs) to same status model.
  - Add hardened-profile live harness run that validates mTLS behavior in expected deployment mode.
- **Strict gates:**
  - [x] Single source-of-truth task status for mTLS in all planning docs.
  - [x] Hardened + mTLS startup and rejection paths covered by automated tests.
  - [x] Operator docs have one canonical mTLS setup flow.
- **Test scenarios:**
  1. Hardened profile + mTLS enabled + valid CA/client cert => success.
  2. Hardened profile + mTLS enabled + missing client cert => denied.
  3. Hardened profile + malformed CA/cert config => startup fails fast.

## P0-02 — Durable external secret backend integration (not only in-memory)
- **Area:** Security / resilience
- **Problem:** Auth persistence improved, but secret backend remains in-memory/demo-oriented.
- **Scope:**
  - Introduce secret backend interface contract + at least one production adapter path (Vault/1Password/KMS shim).
  - Keep in-memory backend as explicit dev-only adapter.
  - Add startup guard/warning when hardened profile runs with in-memory secrets backend.
- **Strict gates:**
  - [ ] Secret retrieval works via external backend adapter.
  - [ ] Hardened profile clearly warns/fails for in-memory secret backend (configurable policy).
  - [ ] Failure modes (backend unavailable, timeout, auth failure) are deterministic and audited.
- **Test scenarios:**
  1. External backend success path for configured secret names.
  2. Backend outage => defined error code + actionable message.
  3. Hardened+in-memory mode => warning/fail according to policy.

## P0-03 — Complete MCP protocol conformance hardening
- **Area:** Security / compatibility
- **Problem:** Good MCP coverage exists, but production claim needs stronger interoperability confidence.
- **Scope:**
  - Expand MCP conformance matrix against target clients.
  - Add strict request/response schema validation for edge/error cases.
  - Add compatibility report artifact in CI.
- **Strict gates:**
  - [ ] Conformance suite includes target MCP client behaviors.
  - [ ] Known edge/error cases have stable expected outputs.
  - [ ] CI publishes conformance summary artifact.
- **Test scenarios:**
  1. Invalid/malformed RPC messages return compliant error shape.
  2. Tool call validation failures preserve protocol semantics.
  3. Batch/stream edge cases match documented behavior.

---

## P1-01 — Extract remaining auth business logic to app layer
- **Area:** Architecture
- **Problem:** Auth handlers still own significant lifecycle logic; limits domain-level testability.
- **Scope:**
  - Add app-layer auth lifecycle service/use-cases.
  - Keep handlers thin: auth gate, decode, delegate, map response.
- **Strict gates:**
  - [ ] Auth lifecycle decisions centralized in app layer.
  - [ ] Handler package no longer contains auth business branching.
  - [ ] Behavior parity preserved (tests + redteam).

## P1-02 — Add hardened deployment profile smoke suite
- **Area:** Security / operations
- **Problem:** Live harness currently runs dev profile for practicality; need hardened-path confidence.
- **Scope:**
  - Add dedicated hardened smoke run (unix socket/TLS/mTLS variants).
  - Include config examples used by smoke tests.
- **Strict gates:**
  - [ ] Hardened smoke test runs in CI profile (full/nightly).
  - [ ] Failures output actionable diagnostics.

## P1-03 — Secret leakage regression suite
- **Area:** Security
- **Problem:** Baseline checks exist, but production requires richer leakage regression cases.
- **Scope:**
  - Add tests for token/secret material across audit logs, error paths, and command output handling.
  - Add negative fixtures and grep-based guardrails in CI.
- **Strict gates:**
  - [ ] No raw secret/token patterns in logs/audit fixtures.
  - [ ] Redaction behavior validated under error and success paths.

## P1-04 — Go-first tooling migration
- **Area:** Developer UX / delivery
- **Problem:** Mixed toolchains increase contributor friction and setup burden.
- **Scope:**
  - Execute `docs/plans/GO-FIRST-TOOLING-MIGRATION-2026-03-08.md`.
  - Replace Python CI/security helpers with Go equivalents where practical.
- **Strict gates:**
  - [ ] Core CI/security automation runnable with Go + shell only.
  - [ ] No new secondary runtime tooling without explicit approval note.

---

## P2-01 — Production runbook quality gate
- **Area:** Usability / operations
- **Scope:**
  - Add “first 30 minutes” deployment checklist and incident quick-reference.
  - Add rollback playbook for TLS/auth/backend config failures.
- **Strict gates:**
  - [ ] New operator can deploy hardened profile from docs only.
  - [ ] Incident checklist includes verification commands + expected outputs.

## P2-02 — Release readiness scoreboard
- **Area:** Delivery governance
- **Scope:**
  - Add a machine-readable readiness matrix (JSON/YAML) consumed by CI.
  - Fail release workflow if any P0 gate is open.
- **Strict gates:**
  - [ ] CI blocks tagged release when production blockers are unresolved.
  - [ ] Status is visible in one file and one workflow output.
