# 0027 - Broker-managed executable resolution

- Status: accepted
- Date: 2026-03-14

## Context
ADR-0026 fixed two important gaps:

1. child processes no longer inherit the full ambient host environment
2. executable allowlisting now matches exact executable identity instead of prefixes

That still left a provenance gap on the broker host. `promptlockd` was executing `command[0]` through the broker process `PATH`, so a same-basename shadow binary earlier in `PATH` could still be selected. The canonical config key was also still named `execution_policy.allowlist_prefixes`, which no longer matched real behavior.

The repository's operator docs and examples are tool-name-centric (`go`, `git`, `echo`), not host-path-centric, so full absolute-path pinning would have been a much larger schema and UX shift than the current architecture needed.

## Decision
1. `execution_policy.exact_match_executables` is the canonical config key for broker-exec executable allowlisting.
2. `execution_policy.allowlist_prefixes` remains a backward-compatible migration alias. If both keys are present, `exact_match_executables` wins.
3. Broker-host execution uses a broker-managed `execution_policy.command_search_paths` list to resolve bare executable names.
4. Path-like `command[0]` values are allowed only when the supplied path itself is inside one of the configured `command_search_paths`.
5. Broker-host child processes receive the same managed `command_search_paths` value as their `PATH` baseline.
6. Resolution remains owned by `internal/app.ControlPlanePolicy` so handlers stay transport-focused.

## Consequences
### Positive
- PATH shadowing is reduced because broker execution no longer depends on the broker process ambient `PATH`.
- The canonical schema now matches actual exact-match behavior.
- Existing name-based docs and examples remain practical because operators can continue allowlisting tool names instead of rewriting all configs around absolute paths.

### Trade-offs
- Trust now explicitly depends on the configured `command_search_paths` directories and the executable entries they expose.
- Package-manager symlink shims reachable from a trusted directory entry are treated as trusted by the broker-managed lookup model.
- Local CLI non-broker execution still uses its minimal ambient `PATH` baseline; this ADR hardens broker-host execution, not every local process-launch path.

## Security implications
- Reduces alternate-binary and PATH-shadowing risk for broker-host execution and host-docker mediation.
- Does not provide cryptographic provenance, hash pinning, signature verification, ownership verification, or immutable mount guarantees for executables.
- If an attacker can replace binaries or symlink entries inside a trusted `command_search_paths` directory, PromptLock will trust that replacement. Host filesystem hardening still matters.

## Follow-up
- Consider an optional stricter mode for absolute-path or hash pinning if operators need stronger provenance than trusted-directory resolution.
- Keep regression coverage for PATH shadowing, path-outside-root rejection, and graceful broker-host shutdown.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
