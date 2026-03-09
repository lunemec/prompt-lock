# 0016 - Execution-surface policy boundaries

- Status: accepted
- Date: 2026-03-08

## Context
`/v1/leases/execute` and `/v1/host/docker/execute` are high-risk surfaces.
Historically, much of command validation/policy logic lived directly in transport handlers.
That made policy drift likely and reduced testability.

## Decision
- Keep inbound handlers transport-focused (auth gate, decode, map status code, encode).
- Centralize execution/egress/host-docker policy in app-layer policy service:
  - `internal/app.ControlPlanePolicy`
  - default implementation: `internal/app.DefaultControlPlanePolicy`
- Composition root (`cmd/promptlockd/main.go`) injects policy implementation into server.
- Handler methods may keep thin wrappers for backward-compatible tests, but wrappers delegate to app policy service.

## Consequences
### Positive
- Single policy implementation path for execute + host-ops checks.
- Easier unit testing and reduced duplicate logic.
- Stronger hexagonal boundary compliance.

### Trade-offs
- Slightly larger app surface area.
- Requires explicit policy wiring in server construction.

## Security implications
- Reduces the chance of policy drift across high-risk execution surfaces.
- Makes security behavior easier to regression-test outside HTTP-specific handlers.

## Follow-up
- Continue migrating remaining non-transport decisions from handlers into app/domain services.
- Extend conformance checks when new adapters/entrypoints are added.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
