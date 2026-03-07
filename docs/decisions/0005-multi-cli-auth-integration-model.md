# 0005 - Multi-CLI authentication integration model

- Status: accepted
- Date: 2026-03-07

## Context
PromptLock should support more than Codex. Different CLIs use different credential mechanisms (env vars, auth files, OS keyrings, OAuth refresh flows).

## Decision
1. Introduce a CLI-agnostic execution pattern:
   - `promptlock exec --lease-request ... -- <tool command>`
2. PromptLock obtains/renews a lease, then injects credentials via the safest supported path for that tool.
3. Prefer **wrapper/adapter** integration over deep CLI patching.
4. Keep tool-specific logic in isolated adapters with explicit risk notes.

## Adapter precedence
For each tool, use this order:
1. Env var injection (preferred where supported)
2. Ephemeral auth file in tmpfs
3. Native keyring bridge helper
4. Direct CLI patch/fork (last resort)

## Security requirements
- No long-lived raw secret mounts into agent containers by default.
- Every credential materialization event must be audit logged.
- Secret values must never appear in logs.
- Lease renewals must be auditable.

## Consequences
- Adds adapter implementation effort.
- Greatly improves portability across Codex/Claude/Gemini/others.
- Reduces maintenance burden compared with maintaining hard forks.
