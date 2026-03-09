# E2E Usability & Operations Gaps (Strict Review) — 2026-03-09

Scope reviewed: real host daemon + untrusted container workflow, operator approval UX, onboarding docs/commands, and failure handling.

Status: Open.

---

## Executive summary (what is missing today)

The core security controls are strong, but end-to-end operator usability has critical friction points:

1. **Auth bootstrap/mint not CLI-complete** for real deployments (still needs API/curl steps).
2. **Docs and examples still contain stale endpoint assumptions** (legacy script paths, old approve endpoint style in places).
3. **No canonical "host + container reality" runbook that is entirely CLI-first**.
4. **No one-shot smoke for real host-daemon + container-agent approval flow** (separate from unit/redteam).
5. **Portability issue remains** for local `make ci` on BSD/macOS due hygiene script behavior.

These are primary blockers for smooth real-world operator adoption even when security gates pass.

---

## Task group A — CLI completeness for real auth lifecycle

### A1 — Add `promptlock auth bootstrap` command
- **Status:** ✅ Completed (2026-03-09)
- **Problem:** Operator must use raw API to create bootstrap token.
- **Deliverable:** `promptlock auth bootstrap --agent <id> --container <id>`
- **Expected output:** bootstrap token + expiry in consistent JSON/text format.
- **Strict gates:**
  - [x] Works against auth-enabled broker.
  - [x] Requires operator auth token and fails cleanly when missing.
  - [x] Covered by CLI integration tests.
- **Verification:**
  - Start daemon, run command, verify token accepted by pair command.

### A2 — Add `promptlock auth pair` command
- **Status:** ✅ Completed (2026-03-09)
- **Problem:** Agent/container pairing currently API-only.
- **Deliverable:** `promptlock auth pair --token <boot> --container <id>`
- **Strict gates:**
  - [x] Returns grant id and expiries.
  - [x] Replay/invalid token errors mapped to deterministic CLI messages.
  - [x] Tests cover success + replay denial + container mismatch.

### A3 — Add `promptlock auth mint` command
- **Status:** ✅ Completed (2026-03-09)
- **Problem:** Session token mint is API-only.
- **Deliverable:** `promptlock auth mint --grant <id>`
- **Strict gates:**
  - [x] Returns session token + expiry.
  - [x] Handles revoked/expired grant with deterministic error text.
  - [x] Tests for success/expired/revoked paths.

### A4 — Optional automation path in `promptlock exec`
- **Problem:** First-run ergonomics for container agents are complex.
- **Deliverable:** `promptlock exec --auto-auth` (or documented equivalent) using provided operator + container identity settings.
- **Strict gates:**
  - [ ] Disabled unless explicit flags/vars are present.
  - [ ] Does not weaken auth or bypass operator approval semantics.
  - [ ] Fully audited bootstrap/pair/mint steps.

---

## Task group B — Single source-of-truth reality runbook

### B1 — Create canonical "Host daemon + container agent" guide
- **Status:** ✅ Completed (2026-03-09)
- **Target:** `docs/operations/REAL-E2E-HOST-CONTAINER.md`
- **Must include:**
  - host daemon startup variants (unix socket, local TCP, TLS)
  - operator terminal using `promptlock approve-queue`
  - container start command (Linux + Docker Desktop notes)
  - CLI-only auth/bootstrap/mint/exec flow (post Task group A)
- **Strict gates:**
  - [x] Zero curl required in canonical flow.
  - [x] Copy-paste commands tested end-to-end.
  - [x] Includes expected output snippets and failure hints.

### B2 — Reconcile README/RUNBOOK/DOCKER examples
- **Status:** ✅ Completed (2026-03-09)
- **Problem:** mixed prototype-era instructions still present.
- **Scope:** align all docs to current endpoint and CLI behavior.
- **Strict gates:**
  - [x] No stale endpoint references.
  - [x] No contradictory startup guidance for hardened TCP behavior.
  - [x] Docs validation includes new runbook file.

---

## Task group C — Real-flow smoke automation

### C1 — Add E2E host+container smoke harness
- **Goal:** verify real approval flow in near-production topology.
- **Deliverable:** scripted smoke harness that:
  1) starts broker,
  2) mints/obtains session via CLI,
  3) issues request from "agent" side,
  4) approves via `approve-queue allow`,
  5) executes command with leased secret.
- **Strict gates:**
  - [ ] Produces machine-readable report under `reports/`.
  - [ ] Included in optional CI profile (nightly/full).
  - [ ] Fails with actionable diagnostics.

### C2 — Add deny-path counterpart
- **Goal:** ensure deny flow behavior is correct and user-friendly.
- **Strict gates:**
  - [ ] Request denied by operator returns deterministic error to agent CLI.
  - [ ] Audit events include deny reason and request id.

---

## Task group D — Portability and developer experience

### D1 — Fix BSD/macOS hygiene compatibility
- **Ref:** existing P1-05 issue (`find -regextype` unsupported).
- **Strict gates:**
  - [ ] `make ci` works on macOS and Linux.
  - [ ] Hygiene detection behavior unchanged.

### D2 — Keep Go-first policy enforceable
- **Problem:** Python still required for some quality gates.
- **Action:** prioritize M1 from `GO-FIRST-TOOLING-MIGRATION-2026-03-08.md`.
- **Strict gates:**
  - [ ] Security/changelog validators available in Go.
  - [ ] CI path does not require Python for standard workflows.

---

## Task group E — Safety/clarity quality gates

### E1 — Endpoint/CLI contract matrix doc
- **Deliverable:** table mapping CLI commands to API endpoints and required auth token type.
- **Strict gates:**
  - [ ] Operator vs agent token requirements are explicit.
  - [ ] Used in troubleshooting section for auth errors.

### E2 — Error-message consistency pass (operator-facing)
- **Goal:** when things fail during real flow, messages tell exact next step.
- **Strict gates:**
  - [ ] Common failures (no session token, wrong endpoint, denied request, backend unavailable) include remediation.
  - [ ] Integration tests assert key error strings.

---

## Proposed execution order
1. **A1 → A2 → A3** (close auth lifecycle CLI gap)
2. **B1 → B2** (publish single canonical run path)
3. **C1 → C2** (automate real-world flow confidence)
4. **D1** (macOS/Linux parity for `make ci`)
5. **D2** (Go-first migration rollout)
6. **E1 → E2** (contract clarity and operator UX polish)

---

## Release readiness impact
- Before this task set: secure internals are strong, real-world operator path is still rough.
- After this task set: project becomes practical for real deployments with predictable onboarding and fewer manual/API-only steps.
