# 0011 - Agent auth: host pairing + refreshable sessions

- Status: accepted
- Date: 2026-03-07

## Context
PromptLock should support long-running containers (including multi-day runs) without frequent re-pairing friction, while preserving tight auth boundaries.

## Decision
Use a two-layer credential model:

1. **Short session token** (minutes)
   - used for normal API calls
   - frequently rotated
2. **Long-lived pairing grant** (hours/days; host-configurable bearer credential)
   - minted at host-orchestrated container start via one-time bootstrap
   - allows silent session re-mint while the grant itself remains active
   - scoped and revocable

## Configurable long-run support
Host config defines:
- session token TTL (e.g., 5–15 min)
- pairing grant idle timeout (e.g., 8h)
- pairing grant absolute max lifetime (e.g., 7d)

This enables day-scale container runtimes while maintaining bounded credentials.

## Required controls
- Bootstrap token is one-time and short-lived.
- Pair-complete validates the bootstrap token against `container_id` + `agent_id`, but subsequent session mint currently uses `grant_id` as a bearer credential rather than re-checking container identity.
- Pairing grant can mint only agent-role sessions (no operator privileges).
- Host can revoke grant/session immediately.
- All pair/mint/refresh/revoke events are audited host-side.

## Consequences
- Supports long-running containers without forcing repeated human pairing for every idle timeout.
- Adds grant lifecycle complexity that must be visible in docs, tests, and audit records.

## Security implications
### Risk: Stolen pairing grant inside compromised container
- Mitigation: grant scoping, idle timeout, absolute expiry, revocation, egress controls.

### Risk: Replay from another container
- Current mitigation is limited: `container_id` is checked during pair-complete, but later session mint does not re-validate container identity. Treat `grant_id` as a sensitive bearer credential and store it accordingly.

### Risk: Silent privilege creep
- Mitigation: strict role model (agent vs operator), endpoint authorization matrix.

### Risk: Long-lived container accumulation
- Mitigation: enforce max grant lifetime, periodic re-pair policy, stale grant sweeper.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
