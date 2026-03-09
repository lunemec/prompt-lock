# Architecture Review Tasks (2026-03-08)

Source: strict architecture review requested by Lukas.
Status: ✅ Completed.

## A-001 — Move policy enforcement out of HTTP handlers
- **Priority:** P1
- **Task:** Refactor auth/policy/execute decision logic currently in `cmd/promptlockd/*` into app-layer services/use-cases.
- **Why:** Preserve hexagonal boundaries and increase testability.
- **Status:** ✅ Completed (2026-03-08)
- **Done when:**
  - [x] Handler files are transport-only (decode/encode/error mapping).
  - [x] Business/security decisions are in `internal/app` and/or core services.
  - [x] Existing behavior parity verified with tests.

## A-002 — Split promptlockd inbound contexts
- **Priority:** P1
- **Task:** Break daemon handler package into bounded contexts (auth, lease, execute, host-ops, meta).
- **Why:** Reduce growth-driven coupling and improve maintainability.
- **Status:** ✅ Completed (2026-03-08)
- **Done when:**
  - [x] New package/module layout documented.
  - [x] Route registration remains explicit and test-covered.
  - [x] No circular dependencies introduced.

## A-003 — Define explicit service interfaces for control-plane capabilities
- **Priority:** P1
- **Task:** Introduce app interfaces for execution policy evaluation, egress checks, and host-docker mediation.
- **Why:** Enable adapter substitution and stronger contract tests.
- **Status:** ✅ Completed (2026-03-08)
- **Done when:**
  - [x] Interfaces live under ports/app boundary.
  - [x] Default implementations injected in main wiring.
  - [x] Unit tests run with mocked implementations.

## A-004 — Add architecture conformance tests
- **Priority:** P1
- **Task:** Add structural tests/linters ensuring inward dependencies only and no forbidden imports across layers.
- **Why:** Prevent gradual erosion of hexagonal architecture.
- **Status:** ✅ Completed (2026-03-08)
- **Done when:**
  - [x] Conformance checks run in CI.
  - [x] Failing example added to prove guardrail effectiveness.
  - [x] Docs explain how to fix violations.

## A-005 — Formalize package boundaries in architecture docs
- **Priority:** P2
- **Task:** Update `docs/architecture/ARCHITECTURE.md` with current package map and ownership of each concern.
- **Why:** Keep implementation and architecture source-of-truth aligned.
- **Status:** ✅ Completed (2026-03-08)
- **Done when:**
  - [x] Diagram or table maps handlers/app/core/adapters.
  - [x] Each public endpoint mapped to owning use-case.
  - [x] Review checklist references this mapping.

## A-006 — Add ADR for execution-surface boundaries
- **Priority:** P2
- **Task:** Write/update ADR describing where execute/host-ops policy belongs (transport vs app vs domain).
- **Why:** Avoid repeated architectural drift debates.
- **Status:** ✅ Completed (2026-03-08)
- **Done when:**
  - [x] ADR accepted in `docs/decisions/`.
  - [x] Code references align with ADR.
  - [x] Follow-up tasks captured for any deferred items.

## A-007 — Standardize error taxonomy across handlers
- **Priority:** P2
- **Task:** Introduce consistent internal error types and HTTP mapping rules.
- **Why:** Improves observability, testing clarity, and client behavior.
- **Status:** ✅ Completed (2026-03-08)
- **Done when:**
  - [x] Shared error mapping utility in inbound adapter.
  - [x] Tests assert stable status-code mapping.
  - [x] Docs include error contract summary.
