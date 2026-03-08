# Strict Re-Review Remediation Tasks (2026-03-08)

Source: strict re-review focused on security, architecture, and usability.
Status: In progress.

## Priority model
- **P0** = blocking for hardened beta credibility
- **P1** = high-value next hardening
- **P2** = important follow-through

---

## P0-01 — Normalize HTTP method semantics (405 consistency)
- **Area:** Usability + API correctness
- **Problem:** Some handlers return `400` for method mismatch while others return `405`.
- **Status:** ✅ Completed (2026-03-08)
- **Scope:**
  - Add explicit `ErrMethodNotAllowed` taxonomy code.
  - Ensure execute + host-docker handlers return `405` for wrong method.
  - Add/adjust tests to prevent regression.
- **Strict gates:**
  - [x] Any method mismatch on registered endpoint returns `405`.
  - [x] Existing `400` validation flows remain `400`.
  - [x] CI + handler tests pass.
- **Test scenarios:**
  1. `GET /v1/leases/execute` => `405`
  2. `GET /v1/host/docker/execute` => `405`
  3. malformed JSON on valid POST => `400`

## P0-02 — Native TLS/mTLS transport profile (deferred but tracked)
- **Status:** 🚧 In progress (phase 1 scaffolding implemented, 2026-03-08)
- **Area:** Security
- **Problem:** No native TLS/mTLS server mode in broker transport path.
- **Scope (phase 1):**
  - Config schema fields for TLS cert/key and optional client CA.
  - TLS listener path in broker with secure defaults.
  - Optional mTLS client cert verification mode.
- **Strict gates:**
  - [ ] Broker can run HTTPS with provided cert/key.
  - [ ] mTLS mode rejects clients without valid cert.
  - [ ] Startup validation fails fast on incomplete TLS config.
- **Test scenarios:**
  1. HTTPS startup success with valid cert/key.
  2. Startup fail with missing key.
  3. mTLS mode rejects unauthenticated client cert.

## P0-03 — Durable backend plan for auth/secret/session stores
- **Area:** Security + resilience
- **Problem:** In-memory stores are non-durable and weak for production operation.
- **Status:** 🚧 In progress (phase 1 auth-store durability implemented, 2026-03-08)
- **Scope (phase 1):**
  - Define persistence adapter interfaces and storage contract.
  - Implement first durable adapter (file/bolt/sqlite or external backend shim).
  - Add migration + failure-mode docs.
- **Strict gates:**
  - [x] Restart preserves grants/sessions where configured.
  - [x] Revocation semantics preserved across restarts.
  - [x] Docs clearly state backend trade-offs.
- **Test scenarios:**
  1. Mint session, restart broker, session still valid (within TTL).
  2. Revoke grant, restart, grant remains revoked.

---

## P1-01 — Extend red-team suite to full endpoint adversarial flows
- **Area:** Security testing
- **Scope:** black-box HTTP abuse harness (auth bypass/replay/policy bypass/egress bypass).
- **Strict gates:**
  - [ ] Includes live broker process harness.
  - [ ] Produces machine-readable findings summary.
  - [ ] Integrated in CI profile (full or nightly).

## P1-02 — Finish transport-thin handlers migration
- **Area:** Architecture
- **Scope:** move remaining policy-like decisions out of `main.go`/handlers into app services.
- **Strict gates:**
  - [ ] Handler files only decode/auth/map/respond.
  - [ ] Business logic fully in app/domain with tests.

## P1-03 — Standardize error mapping across all handlers
- **Area:** Usability + architecture
- **Scope:** move all inbound handlers to shared mapper (no mixed `http.Error` style).
- **Strict gates:**
  - [ ] Stable status semantics per contract.
  - [ ] Contract doc + tests reflect final taxonomy.

---

## P2-01 — Operator guidance quality pass
- **Area:** Usability
- **Scope:** add “denial remediation hints” to policy errors and docs.
- **Strict gates:**
  - [ ] Every common deny path has actionable next-step guidance.
  - [ ] Docs include concrete allowlist/policy examples.
