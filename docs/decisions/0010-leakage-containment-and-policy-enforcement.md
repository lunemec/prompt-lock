# 0010 - Leakage containment and policy enforcement model

- Status: accepted
- Date: 2026-03-07

## Context
If a process receives plaintext secrets, full prevention of intentional leakage is not guaranteed. PromptLock requires defense-in-depth containment and strong observability.

## Decision
Use layered controls:

1. **Non-disclosure defaults**
   - no raw secret-return APIs in standard mode.
2. **Capability-first execution model**
   - `promptlock exec` / intent-driven execution instead of direct secret reads.
3. **Lease-context binding**
   - restrict secret use to approved invocation/session context.
4. **Runtime containment**
   - egress restrictions, hardening profile, minimal writable paths.
5. **Detection + audit**
   - detailed host-side audit trail for request/approve/access/execute/expiry/renewal.
6. **Break-glass flow**
   - explicit, elevated approval for high-risk secret export operations.

## Consequences
- Security posture depends on layering multiple controls rather than assuming any single enforcement point is sufficient.
- Operators must treat runtime hardening and audit operations as part of the product, not as optional add-ons.

## Security implications
- Reduces practical exfiltration risk and improves incident forensics.
- Does not claim absolute prevention once plaintext is available in-process.
- Security posture depends on combining PromptLock controls with container/network hardening.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
