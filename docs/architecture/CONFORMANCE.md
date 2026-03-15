# Architecture Conformance Checks

Run:

```bash
make arch-conformance
```

This guard ensures inward-only dependencies for key layers.

## What it checks
- `internal/core` must not import adapter or transport packages.
- `internal/app` must not import `cmd/promptlockd`.
- `internal/app` must not call `os.Environ()` directly; ambient process state has to be injected from a composition-root or adapter boundary.
- `cmd/promptlockd/*handler*.go` must not construct control-plane policy/use-case runners inline; handlers stay transport-only while wiring happens in the control-plane boundary.
- The CLI transport selector must fail closed when local role sockets are missing instead of silently downgrading to implicit localhost TCP.
- Agent and operator route registration must stay separated.
- Mutating approval endpoints must reject malformed JSON instead of mutating state.

## Failing example (intentional anti-pattern)

```go
// BAD: core importing outbound adapter
import _ "github.com/lunemec/promptlock/internal/adapters/audit"
```

This should fail conformance because core/domain logic must remain adapter-agnostic.

## How to fix violations
- Move adapter-specific behavior behind a port interface.
- Keep policy/use-case logic in `internal/app` or `internal/core`.
- Keep transport details in inbound adapter packages (`cmd/*`).
