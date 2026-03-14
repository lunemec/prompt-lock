# 0021 - External request/lease state backend path

- Status: accepted
- Date: 2026-03-10

## Context
PromptLock durability hardening previously depended on local file persistence (`state_store_file`) plus single-writer locks. Some operators needed request/lease state to live outside the local host, even though the current request/lease transition model does not provide concurrency-safe multi-writer semantics.

## Decision
1. Add a new state backend configuration block:
   - `state_store.type=file|external`
   - `state_store.external_url`
   - `state_store.external_auth_token_env`
   - `state_store.external_timeout_sec`
2. Keep `state_store.type=file` as default, preserving existing local file behavior.
3. Add an outbound adapter (`internal/adapters/externalstate`) implementing request/lease store ports over HTTP endpoints:
   - `PUT /v1/state/requests/{id}`
   - `GET /v1/state/requests/{id}`
   - `GET /v1/state/requests/pending`
   - `PUT /v1/state/leases/{token}`
   - `GET /v1/state/leases/{token}`
   - `GET /v1/state/leases/by-request/{request_id}`
4. Enforce fail-closed behavior for backend outages by classifying external state errors as store-unavailable and returning `503` on request/lease operations.
5. For non-dev profiles with `state_store.type=external`, require:
   - `https://` backend URL
   - non-empty auth token env value at startup.
6. Narrow the support claim: `state_store.type=external` is a durability/availability adapter, not a concurrency-safe distributed state protocol.

## Consequences
- Operators can move request/lease persistence out of the local host process.
- Local file persistence remains available and backward-compatible.
- Runtime dependency on external state backend health is explicit; outages block request/lease flows with `503` until resolved.
- PromptLock now defines an external state API contract that operators must provide.
- Multi-writer or multi-node correctness remains open work until the app/store contract gains atomic transition semantics.

## Security implications
- Non-dev HTTPS enforcement reduces MITM/tampering risk for state reads/writes.
- Token-env requirement prevents accidental unauthenticated external state traffic in non-dev profiles.
- Fail-closed `503` behavior avoids silent lease issuance/approval inconsistency when backend connectivity is degraded.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
