# Discovery 001 — PromptLock requirements clarification

- Status: completed
- Date: 2026-03-07
- Owner: Lukas + Clawie

## Problem statement (draft)
Agents need controlled access to sensitive secrets without long-lived broad mounts, while preserving autonomous developer workflows.

## Discovery goals
1. Define MVP scope and non-goals.
2. Define trust boundaries and threat model.
3. Define approval UX and lease policy.
4. Define audit/compliance requirements.
5. Define integration path with codex-docker.

## Initial assumptions (to confirm)
- Human approval is required for lease issuance.
- Lease can include multiple secrets and fixed TTL.
- Host-side audit trail is mandatory.
- Hexagonal architecture + high test coverage are mandatory.
- PromptLock will be dockerized.

## Clarification questions
### A) Users and workflows
1. Primary users for v1: only you (single operator) or team/multi-operator?
2. Agent identities: free-form agent IDs or authenticated service identities?
3. Do you need both CLI and chat-based approvals in v1?

### B) Approval policy
4. Default max TTL for v1 (e.g., 15/30/60 min)?
5. Should leases be one-time use per secret, or reusable until expiry?
6. Need emergency override/break-glass flow in v1?

### C) Secret model
7. Which first secrets must be supported (top 5)?
8. Should secrets be grouped by project/environment (dev/stage/prod)?
9. Do we allow wildcard requests (e.g., project/*) or only explicit names?

### D) Security and audit
10. Required audit retention period?
11. Need tamper-evident logs in v1 (hash-chain/signing) or v1.1?
12. Any compliance targets now (SOC2/ISO/GDPR) or later?

### E) Runtime and integrations
13. Preferred implementation language for v1: Go (recommended) confirmed?
14. First deployment target: local Docker Compose only, or k8s-ready from day 1?
15. For codex-docker integration, should safe mode be default-on?

## Proposed MVP acceptance criteria (draft)
- Agent requests N secrets for N minutes with reason + task_id.
- Human approves/denies request.
- Approved lease can access only approved secrets until expiry.
- Host-side audit logs for request/approve/deny/access/expiry.
- Validation gates pass (security + changelog + docs).

## Decisions captured
Final v1 requirements are captured in `docs/decisions/0002-v1-requirements.md`.

## Notes
Record future requirement changes as new ADR updates in `docs/decisions/`.
