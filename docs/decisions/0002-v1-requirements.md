# 0002 - PromptLock v1 requirements baseline

- Status: accepted
- Date: 2026-03-07

## Context
Discovery inputs from project owner clarified MVP scope for OSS, agent-agnostic usage with host-CLI approval flow.

## Decision

### Product scope
1. PromptLock is OSS and agent-agnostic (any agent can integrate).
2. Approval channel in v1: host CLI only (plus optional watcher process).

### Lease policy
3. Default lease TTL: 5 minutes.
4. TTL range is user-configurable via host-side config file.
5. Lease is reusable until expiry.
6. Every secret access under an active lease must be logged.

### Secret scope model
7. Explicit secret names only in v1 (no wildcards/groups).
8. Initial focus: containerized env-secret access flows, starting with Codex-related access concerns.

### Audit/logging
9. Detailed logging in v1 is sufficient.
10. Log retention policy is user-managed (tool does not enforce retention in v1).

### Implementation and deployment
11. Implementation language: Go.
12. First deployment target: Docker Compose, integrating with codex-docker workflow.

## Consequences
- Simpler MVP with strong operator control.
- No chat approval complexity in v1.
- Explicit secret requests reduce accidental over-scope.
- Additional policy/compliance controls (tamper-evidence, retention enforcement) remain future enhancements.

## Security implications
- 5-minute default TTL lowers blast radius.
- Reusable lease improves usability but increases need for precise access audit trails.
- Explicit secret naming limits broad secret exposure.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
