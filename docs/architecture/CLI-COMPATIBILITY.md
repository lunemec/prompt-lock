# CLI Compatibility Matrix (Draft)

This matrix tracks how PromptLock can integrate with major coding/agent CLIs.

| Tool | Preferred auth path | Integration approach | Current status | Risk notes |
|---|---|---|---|---|
| Codex CLI | env key or auth cache file/keyring | wrapper + config-driven adapter | discovery | launcher is JS shim to native binary; avoid direct patch unless required |
| Claude CLI | likely env key / config | wrapper + env adapter | discovery | confirm exact auth file/env behavior |
| Gemini CLI | env key | wrapper + env adapter | discovery | low complexity if env-only |
| OpenClaw | config + channel creds | wrapper + host-side policy guard | discovery | avoid over-broad credential exposure to agent runtime |

## General strategy
- Build one PromptLock execution wrapper contract.
- Implement per-tool adapters incrementally.
- Keep adapter behavior and risk model documented and testable.

## Validation scenarios for each tool
1. Lease request + approval
2. Successful command run with injected creds
3. Lease expiry mid-run behavior
4. Lease renewal behavior
5. Audit trail completeness (request/approve/access/expiry/renewal)
