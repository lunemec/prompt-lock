# 0030 - Single `promptlock` CLI surface with daemon lifecycle subcommands

- Status: accepted
- Date: 2026-03-18

## Context
PromptLock currently exposes two top-level binaries in operator workflows:
- `promptlock` (client/operator UX: `watch`, `exec`, `auth`, `setup`, ...)
- `promptlockd` (broker daemon)

This separation is architecturally useful, but it creates ergonomic friction in daily use:
- operators commonly need multiple terminals and two executable names,
- common local/internal flows feel more complex than they need to be,
- new users interpret the split as two different products rather than one system.

For current project use, the dominant use case is internal operation and contributor workflows rather than long-lived public compatibility constraints.

## Decision
1. Move to a single user-facing CLI surface under `promptlock`.
2. Introduce daemon lifecycle commands under the main CLI namespace:
   - `promptlock daemon start`
   - `promptlock daemon stop`
   - `promptlock daemon status`
3. Keep the broker/daemon as a separate runtime component internally (process and API boundaries remain), while presenting one primary CLI entrypoint externally.
4. Add a convenience integrated operator flow for interactive approval with daemon bootstrap (for example, `promptlock watch --spawn-daemon`) so operators can run watch-first workflows without manually orchestrating multiple terminals.
5. Treat standalone daemon operation as still supported for advanced/internal scenarios.
6. Mark direct `promptlockd` invocation as compatibility-oriented/deprecated in docs once the unified flow lands, with removal timing decided later.

## Consequences
- Operator UX improves: one CLI name and discoverable daemon subcommands.
- Internal architecture remains stable: daemon and client concerns stay separated behind the CLI surface.
- Existing scripts using `promptlockd` will need migration guidance when deprecation messaging is introduced.
- Test coverage must expand to include daemon subcommand behavior and combined watch/daemon bootstrap flow.

## Security implications
- This decision is UX/operability focused and does not change core authorization boundaries (operator approval, lease TTL, policy enforcement).
- Convenience flows that auto-start the daemon must preserve hardened defaults and never silently relax security profile or socket-role separation.
- If integrated flows spawn daemon processes, ownership/lifecycle semantics must be explicit to prevent orphaned privileged processes.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
