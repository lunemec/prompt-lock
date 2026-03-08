# Control-plane model for codex-docker + PromptLock

## Future reference (recommended operating model)

Use **codex-docker as a restricted worker runtime** and **PromptLock as the privileged control plane**.

### Worker container (codex-docker)
Default-deny sensitive capabilities:
- no host `docker.sock` bind
- no unrestricted internet egress (restricted/offline by default)
- no raw `.env` mounts by default

### PromptLock (control plane)
Agent requests privileged capabilities through PromptLock instead of direct access:
- host Docker operations (policy + operator approval)
- network/internet exceptions (policy + operator approval)
- secret-dependent execution via lease model

PromptLock then:
1. evaluates policy
2. asks human approval when required
3. executes/permits scoped action
4. writes full host-side audit trail

## Why this model
- least privilege in agent runtime
- lower prompt-injection blast radius
- explicit human oversight for high-risk actions
- better forensics and governance

## Principle
**Deny in container by default; grant through PromptLock by intent and time-bound policy.**
