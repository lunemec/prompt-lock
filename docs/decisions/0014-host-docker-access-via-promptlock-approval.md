# 0014 - Host Docker access via PromptLock approval (no direct docker.sock bind)

- Status: accepted
- Date: 2026-03-08

## Context
Directly binding `/var/run/docker.sock` into agent containers gives broad host control and high exfiltration/escape risk.

## Decision
- PromptLock should provide a host-mediated Docker operation approval path as an alternative to direct docker socket mounting.
- Agent requests Docker capability intent (example: `docker_build`, `docker_ps`, `docker_compose_up`) via PromptLock.
- Host/operator approves scoped operation and PromptLock executes it on the host side under policy constraints.

## Required controls
- No default docker.sock bind to agent containers.
- Explicit allowlist of Docker subcommands/flags per intent.
- Human approval for high-risk Docker operations.
- Full host-side audit trail (request/approve/execute/result).

## Consequences
- Reduced privilege exposure to agent runtime.
- Slightly higher orchestration complexity.
- Better forensic visibility and policy control.

## Security implications
- Significantly lowers risk of container-to-host privilege expansion compared with raw docker.sock mounting.
- Requires strict command-policy hardening to avoid policy bypass.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
