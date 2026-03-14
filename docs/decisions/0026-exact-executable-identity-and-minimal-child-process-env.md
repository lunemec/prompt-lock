# 0026 - Exact executable identity and minimal child-process environment

- Status: accepted
- Date: 2026-03-14

## Context
Strict review found two execution hardening gaps:

1. Broker-side and local CLI execution inherited the full ambient process environment via `os.Environ()`, which leaked unrelated host secrets into child processes.
2. Broker-exec allowlist checks used prefix matching, so `goevil` or `git-backdoor` could pass when `go` or `git` were allowlisted.

The existing `redacted` output mode also risked being read as stronger containment than it actually provides.

## Decision
- Child processes launched by PromptLock receive only a minimal baseline runtime environment plus explicitly leased secrets.
- The built-in baseline is limited to `PATH`, `HOME`, `TMPDIR`, `TMP`, `TEMP`, `SYSTEMROOT`, `COMSPEC`, `PATHEXT`, and `USERPROFILE`.
- Broker and local CLI exec paths both share the same environment-construction rules.
- `execution_policy.allowlist_prefixes` remains the compatibility config key, but matching is exact executable identity after basename normalization rather than prefix matching.
- Hardened documentation must describe `redacted` output mode as best-effort log hygiene only, not as a strong secret-exfiltration boundary.

## Consequences
### Positive
- Ambient non-leased secrets are no longer inherited by child processes by default.
- Common toolchains (`go`, `git`, `make`, `npm`, `node`, `python`, `pytest`) keep the baseline variables they typically need for safe execution.
- Executable-name bypasses like `goevil` and `git-backdoor` are rejected deterministically.

### Trade-offs
- Some workflows that depended on unrelated ambient environment variables now need explicit migration to leased secrets, wrapper `--env`, or host-level tool configuration.
- At the time of this ADR, the legacy config key name `allowlist_prefixes` was still semantically misleading. ADR-0027 later completed that schema cleanup with `execution_policy.exact_match_executables`.

## Security implications
- Reduces secret exposure from unrelated host runtime state during child-process execution.
- Removes a straightforward execution-policy bypass path based on prefixed executable names.
- Clarifies that output redaction does not replace stronger controls such as output suppression, least-privilege commands, and egress/network hardening.

## Follow-up
- The schema cleanup and broker-managed executable resolution follow-up was implemented in ADR-0027.
- Keep regression coverage for minimal env construction and exact executable-name enforcement on both broker and local exec paths.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
