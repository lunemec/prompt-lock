# 0006 - Intent API and execute-with-secret injection model

- Status: accepted
- Date: 2026-03-07

## Context
Agents usually request outcomes ("run tests", "publish package"), not specific secret names. For security and usability, PromptLock needs a low-friction model that does not normalize raw secret retrieval.

## Decision
1. Add an **intent-based API** as the default integration model.
2. Keep explicit secret-name requests as a secondary/power path.
3. Use `promptlock exec` as the primary UX for agents/devs.

## Intent model
- Agent requests capability intent (example: `run_tests`, `publish_npm`, `push_git`, `run_e2e`).
- Host policy maps intent -> allowed secret set.
- Human approves one lease for the bounded intent + TTL.
- Lease is reusable until expiry according to policy.

## Execute-with-secret model
- PromptLock injects env values only into the child process being executed.
- Secret material is not returned to caller by default.
- Secret values are never printed to stdout/stderr by PromptLock.

## Consequences
- Lower friction for autonomous workflows.
- Better alignment with agent behavior and developer ergonomics.
- Reduced accidental over-request of broad secret sets.

## Security implications
- Reduces exposure from direct secret retrieval APIs.
- Keeps approvals semantically meaningful (intent-level).
- Requires robust policy mapping and audit coverage.
