# 0011 - Agent auth: host pairing + idle-resilient sessions

- Status: accepted
- Date: 2026-03-07

## Context
PromptLock should support long-running containers (including multi-day runs) without frequent re-pairing friction, while preserving tight auth boundaries.

## Decision
Use a two-layer credential model:

1. **Short session token** (minutes)
   - used for normal API calls
   - frequently rotated
2. **Long-lived pairing grant** (hours/days; host-configurable)
   - minted at host-orchestrated container start via one-time bootstrap
   - allows silent session re-mint after idle expiry
   - scoped and revocable

## Configurable long-run support
Host config defines:
- session token TTL (e.g., 5–15 min)
- pairing grant idle timeout (e.g., 8h)
- pairing grant absolute max lifetime (e.g., 7d)

This enables day-scale container runtimes while maintaining bounded credentials.

## Required controls
- Bootstrap token is one-time and short-lived.
- Pairing grant is bound to container identity + agent_id (and optionally workdir fingerprint).
- Pairing grant can mint only agent-role sessions (no operator privileges).
- Host can revoke grant/session immediately.
- All pair/mint/refresh/revoke events are audited host-side.

## Security risks and mitigations
### Risk: Stolen pairing grant inside compromised container
- Mitigation: grant scoping, idle timeout, absolute expiry, revocation, egress controls.

### Risk: Replay from another container
- Mitigation: bind grant to container identity/session binding metadata.

### Risk: Silent privilege creep
- Mitigation: strict role model (agent vs operator), endpoint authorization matrix.

### Risk: Long-lived container accumulation
- Mitigation: enforce max grant lifetime, periodic re-pair policy, stale grant sweeper.
