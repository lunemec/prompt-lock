# Auth endpoints (foundation)

This is the initial auth foundation for pairing + idle-resilient session minting.

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
{ "session_id": "sess_..." }
```

## Session-protected endpoints
When `auth.enable_auth=true`, core agent endpoints require:
`Authorization: Bearer <session_token>`

Examples:
- `/v1/intents/resolve`
- `/v1/leases/request`
- `/v1/requests/status`
- `/v1/leases/by-request`
- `/v1/leases/access`

Operator endpoints require operator token:
- `/v1/requests/pending`
- `/v1/leases/approve`
- `/v1/leases/deny`
- `/v1/auth/bootstrap/create`
- `/v1/auth/revoke`

## Current status
- Foundation implemented in memory store and handlers.
- Initial endpoint-level authz enforcement implemented (operator token + session token).
- Remaining hardening: stronger actor attribution in audit events and transport security defaults.
