# CLI Unification Plan (`promptlock` + daemon UX)

Updated: 2026-03-18
Status: planned (not implemented)
Decision reference: `docs/decisions/0030-single-cli-surface-with-daemon-subcommands.md`

## Goal
Deliver one operator-facing CLI (`promptlock`) while preserving current daemon/client runtime boundaries.

## Repository findings (current state)
- Top-level client commands are dispatched in `cmd/promptlock/main.go`.
- Watch UX and queue actions are implemented in `cmd/promptlock/watch.go`.
- Daemon implementation entrypoint is separate in `cmd/promptlockd/main.go`.
- User docs and quickstart currently instruct direct daemon startup via `go run ./cmd/promptlockd`.
- Existing ADR `0023` already established `watch` as the operator-first approval surface.

## Scope for implementation phase
1. Add `daemon` command namespace to `cmd/promptlock`:
   - `daemon start`
   - `daemon stop`
   - `daemon status`
2. Implement daemon process ownership model for `daemon start`:
   - PID/state file strategy in setup instance directory,
   - explicit behavior for already-running daemon,
   - explicit termination semantics for `daemon stop`.
3. Extend watch UX with convenience startup flag:
   - `promptlock watch --spawn-daemon` (name final during implementation),
   - spawn only when daemon is not running,
   - avoid killing externally managed daemon on watch exit.
4. Update docs/help text to make `promptlock` the primary entrypoint.
5. Keep backward compatibility path for direct `promptlockd` during transition, with deprecation notice once stable.

## Non-goals (for this phase)
- Removing `cmd/promptlockd` entirely.
- Breaking hardened dual-socket local-only security model.
- Expanding to remote TCP/TLS transport paths.

## Risks and mitigations
- Risk: unclear daemon ownership when auto-started by watch.
  - Mitigation: track ownership marker and only stop what the current process started (if at all).
- Risk: docs drift during transition.
  - Mitigation: update README, operations docs, and command help in the same PR.
- Risk: regression in existing operator scripts.
  - Mitigation: keep compatibility path and add explicit migration notes.

## Proposed execution order
1. Command grammar + help text scaffolding in `cmd/promptlock`.
2. Daemon status detection primitive and tests.
3. Start/stop implementation with state ownership tests.
4. Watch bootstrap flag and lifecycle rules.
5. Documentation pass and quickstart alignment.
6. Full validation (`make validate-final`).
