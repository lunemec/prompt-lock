# ARCHITECTURE

## Critical requirement
This tool follows **hexagonal architecture**.

## Dependency rule
- Inward dependencies only: adapters depend on core/app ports.
- `internal/core` must not import adapters or transport handlers.
- `internal/app` orchestrates use-cases and policy services without transport coupling.

Conformance check:

```bash
make arch-conformance
```

## Current package map

| Layer | Packages | Responsibility |
|---|---|---|
| Inbound adapters | `cmd/promptlockd/*_handler.go`, `cmd/promptlock-mcp/*` | HTTP/MCP request decoding, auth gates, response mapping |
| App/use-cases | `internal/app/*` | Lease lifecycle orchestration, control-plane policy interfaces/implementations |
| Core domain | `internal/core/domain` | Lease/policy invariants and pure business rules |
| Ports | `internal/core/ports` | Interfaces for stores/audit/clock boundaries |
| Outbound adapters | `internal/adapters/audit`, `internal/adapters/memory`, `internal/adapters/externalstate` | Audit sink and storage implementations |
| Composition root | `cmd/promptlockd/main.go` | Wiring config + adapters + app services |

## Endpoint ownership map

| Endpoint group | Handler layer | Owning use-case/policy |
|---|---|---|
| `/v1/leases/request`, `/approve`, `/deny`, `/cancel`, `/access`, `/by-request` | lease handlers | `internal/app.Service` |
| `/v1/requests/pending` | pending handler | `internal/app.Service` |
| `/v1/leases/execute` | execute handler | `internal/app.ControlPlanePolicy` + `internal/app.Service` |
| `/v1/auth/*` | auth handlers | auth store + authz/rate-limit controls |
| `/v1/meta/*`, `/v1/intents/*` | meta handlers | configuration + intent registry |
| `/v1/host/docker/execute` | host-ops handler | `internal/app.ControlPlanePolicy` |

## Review checklist (for PRs)
- Is handler code transport-only (decode/map/respond) with no policy duplication?
- Are new policy decisions in app/core rather than inline in handlers?
- Are adapters wired in the composition root rather than lazily constructed or swapped from handlers?
- Do config and policy names match enforced semantics, and if a legacy compatibility name remains, is the mismatch documented with cleanup tracked in `docs/plans/BACKLOG.md`?
- Does `internal/core` stay free of transport/adapters imports?
- Did you update tests for both allow and deny paths?
- Did you run `make arch-conformance` and `make ci`?

## Security architecture notes
- Host-side audit sink is mandatory and isolated from agent-writable paths.
- Lease issuance and access checks must remain deterministic and testable at domain/app layers.
- Execution and host-ops policy enforcement belongs in app-layer policy services; handlers enforce auth and map HTTP errors.
