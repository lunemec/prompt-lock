# PromptLock Secret Lease Contract

## 1) Request lease
`POST /v1/leases/request`

Payload:
```json
{
  "agent_id": "ralph-r1",
  "task_id": "TASK-1001",
  "intent": "run_tests",
  "reason": "Run e2e verification",
  "ttl_minutes": 20,
  "secrets": ["github_token", "npm_token"],
  "env_path": ".env",
  "command_fingerprint": "sha256:...",
  "workdir_fingerprint": "sha256:..."
}
```

Response:
```json
{
  "request_id": "req_...",
  "status": "pending"
}
```

Notes:
- The request payload accepts `env_path`; the broker computes `env_path_canonical` itself before operators review the request.
- Legacy clients that still send `env_path_canonical` are tolerated for compatibility, but the value is ignored and never trusted as approval input.

## 2) Human decision
### Approve
`POST /v1/leases/approve?request_id=<id>`

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
`POST /v1/leases/deny?request_id=<id>`

Payload:
```json
{ "reason": "Scope too broad" }
```

### Agent cancel (session-owned pending requests)
`POST /v1/leases/cancel?request_id=<id>`

Payload:
```json
{ "reason": "mcp notification cancelled" }
```

Notes:
- Requires agent session auth when auth is enabled.
- Request must belong to the same agent identity that created it.
- Cancels only `pending` requests (status is set to `denied`).

## 3) Approval/status helpers
- `GET /v1/requests/status?request_id=<id>`
- `GET /v1/leases/by-request?request_id=<id>`

## 4) Access secret with lease
`POST /v1/leases/access`

Payload:
```json
{
  "lease_token": "lease_...",
  "secret": "github_token",
  "command_fingerprint": "sha256:...",
  "workdir_fingerprint": "sha256:..."
}
```

Response:
```json
{
  "secret": "github_token",
  "value": "<redacted in logs>"
}
```

## 5) Execute with lease-scoped secrets (preferred for hardened mode)
`POST /v1/leases/execute`

Payload:
```json
{
  "lease_token": "lease_...",
  "intent": "run_tests",
  "command": ["go", "test", "./..."],
  "secrets": ["github_token", "npm_token"],
  "command_fingerprint": "sha256:...",
  "workdir_fingerprint": "sha256:..."
}
```

Response:
```json
{
  "exit_code": 0,
  "stdout_stderr": "..."
}
```

Notes:
- Hardened profile requires `intent` and rejects raw shell-wrapper entrypoints such as `bash`, `sh`, and `zsh`.
- Execute-time `intent` must exactly match the approved request/lease intent. PromptLock does not widen egress policy from a different caller-supplied execute payload.
- Broker egress checks are defense-in-depth command validation, not runtime packet mediation. Direct network clients such as `curl`, `wget`, and `fetch` are denied when no inspectable destination is present in argv.
- Child processes receive only an explicit minimal baseline environment plus leased secrets; the ambient broker or CLI environment is not forwarded by default.
- `output_security_mode=redacted` performs token-aware best-effort masking for common bearer and env-style secret shapes. It is not a strong output-containment or secret-exfiltration boundary. Use `none` when output should be suppressed.
- When a request carries `env_path`, execution resolves leased secrets from that approved `.env` file instead of the broker process environment. The broker stores both the original path and a canonicalized path, and execute-time secret access fails closed if the canonical path no longer matches.

## `env_path` trust boundary
- `--env-path` is agent-supplied approval context. The broker canonicalizes it on request and stores both the original and canonical path for operator review.
- `env_path` is constrained to `PROMPTLOCK_ENV_PATH_ROOT`. Traversal and symlink escapes outside that root are rejected.
- If `PROMPTLOCK_ENV_PATH_ROOT` is unset, the broker currently falls back to its current working directory. That is only safe when the broker starts from a host-owned directory outside agent-controlled workspace mounts.
- Broker startup fails closed if the chosen `PROMPTLOCK_ENV_PATH_ROOT` (or fallback working directory) cannot be initialized as an env-path source root.
- `env_path` disables active-lease reuse for matching requests because the approved file path becomes part of the trust boundary.

## Enforcement rules
- Default deny.
- Lease must be unexpired.
- Lease may access only listed secrets.
- `ttl_minutes` is capped by broker policy.
- Secret access is audit-gated before PromptLock reads secret material, and successful reads emit `secret_access` with agent/task/secret/timestamp context.

## Critical requirement: host-side audit trail
This is a critical requirement for production use.

- Audit records must be written on the host (outside agent workspace/container writable paths).
- Required events: request created, approved, denied, `secret_access_started`, `secret_access`, lease expired/revoked.
- Records must include: timestamp, agent_id, task_id, requested secrets, decision actor, TTL, and outcome.
- Audit storage should be host-owned and verifiable through the local hash chain.
- Agent/container users must not be able to modify or delete historical audit logs.

## UX policy
To avoid excessive prompts:
- one request may include multiple secrets,
- lease duration may be N minutes,
- encourage minimal required scope (least privilege).

## Suggested default limits (v1 baseline)
- Default TTL: 5 min
- TTL range: configurable by host operator in host-side config
- Max secrets per request: 5 (recommended initial default)
- High-risk secrets may require re-approval regardless of existing lease (policy extension)

## v1 scope choices
- Explicit secret names only (no wildcard/group requests)
- Leases are reusable until expiry
- Every access under a lease must be audit-gated before the broker reads secret material, and successful reads must still emit `secret_access`

## Error contract summary
- `400` bad request: malformed JSON, missing required fields, invalid method for endpoint.
- `401` unauthorized: missing/invalid auth token or inactive/revoked session.
- `403` forbidden: policy-denied operations (execution policy, egress policy, host-ops policy, lease/secret scope mismatch).
- `429` too many requests: auth rate-limit threshold reached.
- `404` not found: unknown request/lease/intent identifiers.
- `503` service unavailable: durability gate closed, audit-gated mutation blocked, or configured state/secret backend unavailable.
- `500` internal error: unexpected execution/runtime failures.

## Later-stage feature: biometric human approval
Planned for a later phase (not in MVP):

- macOS approval via Apple biometrics (Touch ID / Face ID through platform auth APIs)
- Windows approval via Windows Hello
- Optional Linux desktop biometric providers where available

Target behavior:
- secret lease approval prompt can require biometric verification for high-risk secrets
- policy can mark which secrets/actions require biometric confirmation vs standard approval
- audit log records biometric-verified approval event (without storing biometric data)
