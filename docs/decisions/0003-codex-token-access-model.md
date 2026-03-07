# 0003 - Codex token access model (v1)

- Status: accepted
- Date: 2026-03-07

## Context
Codex-like tools may need access to credentials for longer-running autonomous tasks.

## Decision
- Keep host-owned source of truth for secrets.
- Agent containers do not get long-lived token files by default.
- Agents request time-bound leases for explicit secret names.
- Leases are reusable until expiry (default 5 minutes, host-configurable range).
- For longer runs, agent renews lease via new approval request.

## Consequences
- Better containment under prompt-injection conditions.
- More approval interactions for very long tasks unless TTL policy is widened by operator.

## Security implications
- Short leases reduce blast radius.
- Renewal events add explicit visibility in audit logs.
