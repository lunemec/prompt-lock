# ARCHITECTURE

## Critical requirement
This tool must follow **hexagonal architecture**.

## Target structure
- `core/domain` — lease rules, policy decisions, validation (no IO/network deps)
- `core/ports` — interfaces for secret store, approval channel, audit sink, clock
- `adapters/inbound` — HTTP/CLI handlers
- `adapters/outbound` — secret backends, host audit writer, notifier integrations
- `app` — orchestration/use-cases wiring ports to adapters

## Dependency rule
- Inward dependencies only: adapters depend on core ports; core never depends on adapters.

## Security architecture notes
- Host-side audit sink is mandatory and isolated from agent-writable paths.
- Lease issuance and access checks must be deterministic and testable at domain layer.
