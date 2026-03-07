# 0007 - TTL bypass prevention and secret non-disclosure defaults

- Status: accepted
- Date: 2026-03-07

## Context
If raw secret values are returned to agent scripts, they can be copied into long-lived environment variables (`ENV_VAR=$(...)`) and bypass TTL/lease constraints.

## Decision
1. **Default mode must be non-disclosure**:
   - no raw secret value return in normal flow.
2. Secret usage should be provided through execute-time injection only.
3. Any explicit secret read/export mode is high-risk and must be separately policy-gated and auditable.

## Required controls
- Lease/token binding to command invocation context.
- Deny use outside approved context.
- Redaction policy for logs and outputs.
- Audit events for request, approve, deny, execute, access, expiry, renewal.
- Host-side audit persistence remains mandatory.

## Optional hardening controls
- Outbound egress allowlists in secure mode.
- Pattern-based detection/denial for suspicious secret-exfil command forms.
- Break-glass flow requiring stronger approval step.

## Consequences
- Stronger enforcement of lease semantics.
- Slightly more complexity in command orchestration.
- Better defense against prompt-injection-assisted exfiltration.
