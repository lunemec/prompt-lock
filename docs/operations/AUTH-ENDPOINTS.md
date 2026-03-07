# Auth endpoints (foundation)

This is the initial auth foundation for pairing + idle-resilient session minting.

## Endpoints

### 1) Create bootstrap token (host/operator path)
`POST /v1/auth/bootstrap/create`

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

Body:
```json
{ "grant_id": "grant_..." }
```

or

```json
{ "session_id": "sess_..." }
```

## Current status
- Foundation implemented in memory store and handlers.
- Endpoint-level authz enforcement is still pending hardening work.
- Next step: require authenticated operator/agent identities per endpoint class.
