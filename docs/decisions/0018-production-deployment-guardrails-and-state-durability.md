# 0018 - Production deployment guardrails and state durability

- Status: accepted
- Date: 2026-03-09

## Context
PromptLock previously allowed compatibility-first startup defaults (`security_profile=dev`) and relied on in-memory request/lease state by default. Auth persistence existed but was plaintext by default. A production-readiness re-review identified four concrete gaps for real deployment posture:

1. Production startup could unintentionally run in dev mode without explicit operator acknowledgment.
2. Request and lease state was not durable across process restart unless re-created externally.
3. Auth store persistence could remain plaintext in non-dev profiles.
4. Secret source adapters did not include a first-class external HTTP-backed retrieval path.

## Decision
1. Add explicit startup opt-in for dev profile:
   - `security_profile=dev` now requires `PROMPTLOCK_ALLOW_DEV_PROFILE=1`.
2. Add durable request/lease state persistence support:
   - new host path setting `state_store_file`.
   - in-memory request/lease adapter can now save/load state from file atomically.
3. Add encrypted auth persistence requirement for non-dev profile when `auth.store_file` is used:
   - configure key env via `auth.store_encryption_key_env` (default `PROMPTLOCK_AUTH_STORE_KEY`).
   - startup fails in non-dev if auth-store encryption key is missing.
4. Add external secret source adapter:
   - `secret_source.type=external`
   - configurable `secret_source.external_url`, `secret_source.external_auth_token_env`, `secret_source.external_timeout_sec`.
5. Add fail-closed runtime durability semantics:
   - auth-store and request/lease persistence write failures now close a durability gate.
   - mutating auth/lease endpoints return `503 Service Unavailable` while gate is closed.
   - failures are explicitly audit-logged (`durability_persist_failed`, `durability_gate_closed`).

## Consequences
- Production mode is now more fail-closed by default.
- Operators must explicitly set startup environment for local dev profile and auth-store encryption.
- Request/lease continuity across restarts is possible with host-backed state file.
- External secret retrieval can be integrated without embedding secret material directly in PromptLock config.
- Runtime persistence faults are no longer warn-only; write paths fail closed until operator remediation.

## Security implications
- Reduces accidental insecure deployment risk by requiring explicit dev-profile opt-in.
- Reduces exposure of persisted auth tokens by enforcing encrypted auth-store persistence in non-dev.
- Improves auditability and continuity by enabling durable request/lease state.
- External secret backend support reduces dependence on in-memory or file-embedded secret material.
- Prevents silent non-durable write acknowledgments by failing mutating flows closed when persistence is unavailable.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
