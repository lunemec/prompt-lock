# PromptLock Secret Lease Contract

## 1) Request lease
`POST /v1/leases/request`

Payload:
```json
{
  "agent_id": "ralph-r1",
  "task_id": "TASK-1001",
  "reason": "Run e2e verification",
  "ttl_minutes": 20,
  "secrets": ["github_token", "npm_token"]
}
```

Response:
```json
{
  "request_id": "req_...",
  "status": "pending"
}
```

## 2) Human decision
### Approve
`POST /v1/leases/{request_id}/approve`

Payload:
```json
{ "ttl_minutes": 20 }
```

Response:
```json
{
  "status": "approved",
  "lease_token": "lease_...",
  "expires_at": "2026-03-07T17:40:00Z",
  "secrets": ["github_token", "npm_token"]
}
```

### Deny
`POST /v1/leases/{request_id}/deny`

Payload:
```json
{ "reason": "Scope too broad" }
```

## 3) Access secret with lease
`POST /v1/leases/access`

Payload:
```json
{
  "lease_token": "lease_...",
  "secret": "github_token"
}
```

Response:
```json
{
  "secret": "github_token",
  "value": "<redacted in logs>"
}
```

## Enforcement rules
- Default deny.
- Lease must be unexpired.
- Lease may access only listed secrets.
- `ttl_minutes` is capped by broker policy.
- Access is audited (agent, task, secret, timestamp).

## Critical requirement: host-side audit trail
This is a critical requirement for production use.

- Audit records must be written on the host (outside agent workspace/container writable paths).
- Required events: request created, approved, denied, secret accessed, lease expired/revoked.
- Records must include: timestamp, agent_id, task_id, requested secrets, decision actor, TTL, and outcome.
- Audit storage should be append-only or tamper-evident.
- Agent/container users must not be able to modify or delete historical audit logs.

## UX policy
To avoid excessive prompts:
- one request may include multiple secrets,
- lease duration may be N minutes,
- encourage minimal required scope (least privilege).

## Suggested default limits
- Max TTL: 30 min (override only with explicit human approval)
- Max secrets per request: 5
- High-risk secrets require re-approval regardless of existing lease

## Later-stage feature: biometric human approval
Planned for a later phase (not in MVP):

- macOS approval via Apple biometrics (Touch ID / Face ID through platform auth APIs)
- Windows approval via Windows Hello
- Optional Linux desktop biometric providers where available

Target behavior:
- secret lease approval prompt can require biometric verification for high-risk secrets
- policy can mark which secrets/actions require biometric confirmation vs standard approval
- audit log records biometric-verified approval event (without storing biometric data)
