# PromptLock auth model (draft)

## Goals
- frictionless for long-running agent containers
- strict separation of agent vs operator privileges
- revocable, auditable credentials

## Flow
1. Host starts container and injects one-time bootstrap token.
2. Agent exchanges bootstrap token for pairing grant (one-time bootstrap burn).
3. Agent uses pairing grant to obtain short session token.
4. Agent uses session token for API calls.
5. If idle/session expires, agent re-mints session token using pairing grant.
6. Host can revoke grants/sessions at any time.

## Lifetime model (configurable)
- session_ttl_minutes (short)
- grant_idle_timeout_minutes
- grant_absolute_max_minutes (supports multi-day runs)

## Role model
- agent role: request/status/capability execution endpoints only
- operator role: approve/deny/revoke/admin endpoints

## Security notes
- Pairing grant is sensitive and must live in tmpfs, not persistent workspace.
- Use strict endpoint authorization; no implicit trust of container-local callers.
- Consider unix socket + mTLS or signed bearer credentials depending deployment mode.
