# Security Defaults Remediation (2026-03-09)

Status: Completed (2026-03-09)  
Owner: PromptLock maintainers  
Source: Follow-up on plaintext-return and auth-boundary review (2026-03-09)

## Priority model
- P0: must-fix security posture gaps with direct exposure risk
- P1: visibility and operator guidance improvements

## Tasks

### P0-01 - Block unsafe unauthenticated non-local TCP startup by default
- Status: ✅ Completed (2026-03-09)
- Problem:
  - Current non-local TCP startup guard is enforced only when auth is enabled.
  - With `auth.enable_auth=false`, broker can start on non-local TCP without TLS/UDS protection.
- Scope:
  - Enforce fail-fast startup when auth is disabled on non-local TCP without TLS/UDS.
  - Keep an explicit insecure override env var for local testing labs.
  - Emit warning + audit event when override is used.
- Success gates:
  - [x] Startup fails by default for `auth=false` + non-local TCP + no TLS/UDS.
  - [x] Explicit override allows startup and is audit-logged.
  - [x] Tests cover default deny + override path.

### P0-02 - Enforce plaintext-return policy independent of auth-mode
- Status: ✅ Completed (2026-03-09)
- Problem:
  - `/v1/leases/access` plaintext-blocking currently depends on `authEnabled`.
  - This can cause policy ambiguity for deployments that disable auth but intend plaintext return to remain off.
- Scope:
  - Apply `allow_plaintext_secret_return` policy regardless of auth mode.
  - Keep existing audit signal for blocked plaintext access attempts.
  - Add tests for auth-disabled + plaintext-disabled behavior.
- Success gates:
  - [x] `/v1/leases/access` denies when `allow_plaintext_secret_return=false` even with auth disabled.
  - [x] Existing auth-enabled deny behavior remains unchanged.
  - [x] Access-policy tests cover both auth states.

### P1-01 - Improve insecure-mode visibility for operators
- Status: ✅ Completed (2026-03-09)
- Problem:
  - Operators may not immediately realize they are running in unauthenticated plaintext-compatible mode.
- Scope:
  - Emit startup warning + audit event when running with `auth=false` and plaintext return enabled.
  - Expose an explicit `insecure_dev_mode` capability flag.
  - Update operations docs with precise override variables and mode semantics.
- Success gates:
  - [x] Startup warning + audit event present for insecure dev mode.
  - [x] `/v1/meta/capabilities` includes `insecure_dev_mode`.
  - [x] Docs reflect the exact behavior and override knobs.

## Execution order
1. P0-01
2. P0-02
3. P1-01
