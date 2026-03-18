# CLI and Endpoint Contract Matrix

Canonical mapping of PromptLock CLI commands to broker endpoints and required token type.

Use this matrix when troubleshooting auth failures or ambiguous endpoint usage.

## Contract matrix

| CLI command | Endpoint(s) used | Required token type | Actor |
|---|---|---|---|
| `promptlock auth bootstrap --operator-token ...` | `POST /v1/auth/bootstrap/create` | Operator token (`PROMPTLOCK_OPERATOR_TOKEN`) | Operator |
| `promptlock auth pair --token ... --container ...` | `POST /v1/auth/pair/complete` | Bootstrap token (in request body) | Agent/container |
| `promptlock auth mint --grant ...` | `POST /v1/auth/session/mint` | Pairing grant ID (in request body) | Agent/container |
| `promptlock auth login --operator-token ...` | `POST /v1/auth/bootstrap/create`, `POST /v1/auth/pair/complete`, `POST /v1/auth/session/mint` | Operator token for bootstrap, then bootstrap token / grant id internally | Operator / agent bootstrap helper |
| `promptlock auth docker-run --operator-token ... --image ...` | Same auth endpoints as `auth login`, then local `docker run` with injected session env | Operator token for bootstrap, then local Docker access on the host | Operator |
| `promptlock watch list` | `GET /v1/requests/pending` | Operator token | Operator |
| `promptlock watch allow <request_id>` | `POST /v1/leases/approve?request_id=...` | Operator token | Operator |
| `promptlock watch deny <request_id>` | `POST /v1/leases/deny?request_id=...` | Operator token | Operator |
| `promptlock exec --intent ... --broker-exec` | `GET /v1/meta/capabilities`, `POST /v1/intents/resolve`, `POST /v1/leases/request`, `GET /v1/requests/status`, `GET /v1/leases/by-request`, `POST /v1/leases/execute` | Agent session token (`PROMPTLOCK_SESSION_TOKEN`) when auth enabled | Agent |
| `promptlock exec --intent ...` (plaintext path) | Same as above, but final step is `POST /v1/leases/access` per secret | Agent session token when auth enabled | Agent |
| MCP `notifications/cancelled` during `tools/call` | `POST /v1/leases/cancel?request_id=...` (best effort cleanup) | Agent session token when auth enabled | Agent |
| `promptlock audit-verify --file ...` | No broker endpoint (local file verification) | N/A | Operator |

## Error remediation map

| Failure text | Likely cause | Exact remediation |
|---|---|---|
| `operator auth required` | Missing/invalid operator token on operator endpoint | Set `--operator-token` (or `PROMPTLOCK_OPERATOR_TOKEN`) to the configured operator token and retry. |
| `agent session token required` or `broker requires session token` | Agent endpoint called without session token while auth is enabled | Mint a session (`auth login`) or use `auth docker-run` so the session env is injected automatically. |
| `request denied` | Operator denied lease request | Review deny reason in `watch`/audit and re-run with corrected reason/intent/secrets. |
| `request not owned by agent` | Agent attempted to cancel another agent's request id | Verify cancellation request id belongs to the same session agent identity that created it. |
| `request_id required` | Wrong endpoint usage or missing query parameter | Use CLI commands rather than manual endpoint calls, or include the documented `request_id` query parameter. |
| `secret backend unavailable` | Secret backend misconfigured/unreachable | Validate `secret_source.type` config and backend-specific settings (`env_prefix`, `file_path`, permissions, availability). |

## Notes

- Auth endpoint details are documented in `docs/operations/AUTH-ENDPOINTS.md`.
- Full host+container walkthrough is documented in `docs/operations/REAL-E2E-HOST-CONTAINER.md`.
- In local hardened mode, CLI commands auto-select sockets by role with no broker flags:
  - operator commands use `/tmp/promptlock-operator.sock`
  - agent commands use `/tmp/promptlock-agent.sock`
  - `auth login` / `auth docker-run` span both sockets and give containers only agent-side transport (`PROMPTLOCK_AGENT_UNIX_SOCKET` on Linux, daemon bridge `PROMPTLOCK_BROKER_URL` on non-Linux desktop Docker)
