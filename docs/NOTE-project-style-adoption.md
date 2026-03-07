# Note for other projects: adopting this agent/docs style

This repository adopts the same lightweight project harness style:
- short `AGENTS.md` as map + hard constraints
- structured `docs/` as source of truth
- progressive disclosure (start small, dive deeper by links)

Use this as a reusable template for other projects.

## Critical constraints in this project
1. Hexagonal architecture is mandatory.
2. Maximum practical test coverage is mandatory.
3. Red-Green-Blue TDD is mandatory (blue = security-focused refactor).
4. Potential security issues must always be surfaced, never silently ignored.

## Reuse checklist for other repos
- Add `AGENTS.md` with non-negotiables.
- Add `docs/{architecture,plans,standards,operations,context}`.
- Define test and security gates in standards.
- Require explicit security findings section in every completion report.
