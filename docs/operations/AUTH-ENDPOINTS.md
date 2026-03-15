# Auth endpoints (foundation)

This is the initial auth foundation for pairing + refreshable session minting.

## Endpoints

### 1) Create bootstrap token (host/operator path)
`POST /v1/auth/bootstrap/create`

Requires operator auth header:
`Authorization: Bearer <operator_token>`

Body:
```json
{ "agent_id": "ralph-r1", "container_id": "container-123" }
```

### 2) Complete pairing (agent path)
`POST /v1/auth/pair/complete`

Body:
```json
{ "token": "boot_...", "container_id": "container-123" }
```

Response includes `grant_id` with idle + absolute expiry timestamps.
Treat `grant_id` as a bearer credential after pair-complete. Current session mint does not re-check `container_id`.
The low-level CLI plumbing commands `promptlock auth pair` and `promptlock auth mint` print raw bearer values for scripting. Prefer `promptlock auth login` or `promptlock auth docker-run` when you do not need to handle those bearer values directly.

### 3) Mint short session from grant (agent path)
`POST /v1/auth/session/mint`

Body:
```json
{ "grant_id": "grant_..." }
```

Response includes `session_token` and expiry.

### 4) Revoke grant/session (operator path)
`POST /v1/auth/revoke`

Requires operator auth header:
`Authorization: Bearer <operator_token>`

Body:
```json
{ "grant_id": "grant_..." }
```

or

```json
{ "session_token": "sess_..." }
```

`session_token` is the canonical revoke field. `session_id` remains a legacy compatibility alias, but operators should not document or automate against it as the primary contract.

## Session-protected endpoints
When `auth.enable_auth=true`, core agent endpoints require:
`Authorization: Bearer <session_token>`

Examples:
- `/v1/intents/resolve`
- `/v1/leases/request`
- `/v1/requests/status`
- `/v1/leases/by-request`
- `/v1/leases/cancel`
- `/v1/leases/access`
- `/v1/leases/execute`

Note:
- `allow_plaintext_secret_return=false` blocks `/v1/leases/access` even when auth is disabled.

Operator endpoints require operator token:
- `/v1/requests/pending`
- `/v1/leases/approve`
- `/v1/leases/deny`
- `/v1/auth/bootstrap/create`
- `/v1/auth/revoke`
- `/v1/host/docker/execute`

## Current status
- Auth foundation, endpoint authz enforcement, actor attribution, and transport safety defaults are implemented.
- Auth endpoint throttling covers bootstrap, pair-complete, session-mint, and revoke flows through the configured `rate_limit_*` settings.
- Durable auth persistence and CLI auth helpers are documented elsewhere in the operations docs and README.
- The canonical CLI-command to endpoint/token mapping is in `docs/operations/CLI-ENDPOINT-CONTRACT-MATRIX.md`.
- Follow-up operator-documentation polish is tracked in `docs/plans/BACKLOG.md`.
