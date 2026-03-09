# AGENTS.md

Purpose: short map + hard rules for this repository.

## Fast start
1. Read `README.md`.
2. Read `docs/README.md`.
3. Read `docs/architecture/ARCHITECTURE.md`.
4. Read `docs/standards/ENGINEERING-STANDARDS.md`.
5. If implementing features or resuming work, read `docs/plans/ACTIVE-PLAN.md` and `docs/plans/BACKLOG.md`.

## Critical engineering rules
- **Hexagonal architecture is mandatory** (ports/adapters, domain core isolated from infra).
- **Maximum practical test coverage is mandatory** for security-critical flows.
- Development style must follow **Red-Green-Blue TDD**:
  - **Red**: write failing test first
  - **Green**: minimal code to pass
  - **Blue**: security-focused refactor/general cleanup while keeping tests green
- Any potential security issue must be explicitly raised in output and plan updates.
- Expose developer/user workflows via **Makefile commands**.
- Prefer the repository’s primary language/toolchain (**Go**) for new tooling and automation; avoid adding secondary runtimes unless absolutely necessary and explicitly approved.
- Significant decisions and requirement changes must be captured in ADRs under `docs/decisions/` and indexed in `docs/decisions/INDEX.md`.
- Keep changelog in Keep-a-Changelog format; new changes go to `[Unreleased]` until release.
- `make validate-final` is the mandatory final validation gate before commit.

## Documentation structure requirements
- `docs/README.md` is the documentation map and must stay aligned with the actual tree.
- `docs/plans/ACTIVE-PLAN.md` is the single canonical run-to-run handoff file for agents.
- `docs/plans/BACKLOG.md` is the single canonical list of open tasks and pending work.
- `docs/plans/initiatives/` is for active multi-step work. `docs/plans/checklists/` is for release/migration gates. `docs/plans/notes/` is for supporting notes. `docs/plans/status/` is for machine-readable state files. `docs/plans/archive/YYYY/` is for completed or superseded plan docs.
- New active planning docs must use stable names. Date-stamped filenames are reserved for archived snapshots and historical records.
- Do not track the same open task in multiple active files with conflicting statuses. Keep canonical status in `BACKLOG.md`; initiative docs may carry extra detail and acceptance criteria.
- When a plan or checklist is completed or superseded, move it to `docs/plans/archive/YYYY/` and update `ACTIVE-PLAN.md` and `BACKLOG.md`.
- When adding or modifying an ADR, update `docs/decisions/INDEX.md` in the same change.

## Completion output required
- Summary of changes
- Tests run + results
- Security findings / concerns
- Files changed
- Follow-up actions

## Knowledge map
- Docs map: `docs/README.md`
- Architecture: `docs/architecture/`
- Plans: `docs/plans/`
- ADR index: `docs/decisions/INDEX.md`
- Standards: `docs/standards/`
- Operations: `docs/operations/`
- Context: `docs/context/`
