# Architecture Conformance Checks

Run:

```bash
make arch-conformance
```

This is the fast boundary-check reference for reviewers and agents.
It complements `docs/architecture/ARCHITECTURE.md`; it is not a second architecture spec.

## What it checks
- `internal/core` must not import transport or adapter packages.
- `internal/app` must not import `cmd/promptlockd`.
- `internal/app` must not call `os.Environ()` directly; ambient process state must be injected from composition or adapter boundaries.
- `cmd/promptlockd/control_plane_wiring.go` must not restore process-global env when boundary injection is missing.
- `cmd/promptlockd/*handler*.go` must stay transport-only and must not construct control-plane runners inline.
- CLI Unix-socket selection must fail closed when the selected path is missing or is not a real Unix socket.
- Agent and operator route registration must stay separated.
- Mutating approval endpoints must reject malformed JSON.

## How to fix violations
- Move adapter-specific behavior behind a port.
- Keep policy and use-case logic in `internal/app` or `internal/core`.
- Keep transport details in inbound adapter packages under `cmd/`.
