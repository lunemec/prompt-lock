# AGENTS.md

Purpose: short map + hard rules for this repository.

## Fast start
1. Read `README.md`.
2. Read `docs/architecture/ARCHITECTURE.md`.
3. Read `docs/standards/ENGINEERING-STANDARDS.md`.
4. If implementing features, read `docs/plans/ACTIVE-PLAN.md`.

## Critical engineering rules
- **Hexagonal architecture is mandatory** (ports/adapters, domain core isolated from infra).
- **Maximum practical test coverage is mandatory** for security-critical flows.
- Development style must follow **Red-Green-Blue TDD**:
  - **Red**: write failing test first
  - **Green**: minimal code to pass
  - **Blue**: security-focused refactor/general cleanup while keeping tests green
- Any potential security issue must be explicitly raised in output and plan updates.

## Completion output required
- Summary of changes
- Tests run + results
- Security findings / concerns
- Files changed
- Follow-up actions

## Knowledge map
- Architecture: `docs/architecture/`
- Plans: `docs/plans/`
- Standards: `docs/standards/`
- Operations: `docs/operations/`
- Context: `docs/context/`
