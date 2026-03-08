# 0015 - Network egress control via PromptLock policy

- Status: accepted
- Date: 2026-03-08

## Context
Prompt injection and autonomous agents can exfiltrate data via outbound network paths.

## Decision
- PromptLock should support policy-driven network egress control integration:
  - allowlist domains/endpoints per intent/profile,
  - support offline/restricted modes,
  - enforce stricter defaults for hardened profile.
- Network policy must be coordinated with runtime/container controls (host firewall/proxy/namespace policy).

## Required controls
- Per-intent egress policy metadata.
- Operator override flow for exceptional network access.
- Audit records for policy denials and granted exceptions.
- Baseline implementation includes command-domain allowlist and deny-substring checks in broker-exec path.

## Consequences
- Better containment of data exfiltration attempts.
- Potentially more operational tuning for legitimate workflows.

## Security implications
- Reduces blast radius under compromised or malicious prompts.
- Must be paired with execution policy controls and output redaction to be effective.
