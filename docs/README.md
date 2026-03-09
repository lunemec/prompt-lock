# Documentation Map

This directory is organized by document type. Agents are expected to preserve this structure rather than adding new flat files ad hoc.

## Read order
1. `README.md`
2. `docs/README.md`
3. `docs/architecture/ARCHITECTURE.md`
4. `docs/standards/ENGINEERING-STANDARDS.md`
5. `docs/plans/ACTIVE-PLAN.md`
6. `docs/plans/BACKLOG.md`
7. Relevant initiative, checklist, note, or ADR for the task at hand

## Layout
- `architecture/` — architecture source of truth and security architecture references
- `compatibility/` — external protocol and client compatibility matrices
- `context/` — product and trust-boundary context
- `decisions/` — ADRs plus `INDEX.md` for the decision log
- `operations/` — runbooks, deployment guides, and operational procedures
- `plans/` — execution state, checklists, initiative docs, and archived plan history
- `standards/` — engineering process and repository rules

## Planning rules
- `docs/plans/ACTIVE-PLAN.md` is the canonical run-to-run handoff for agents.
- `docs/plans/BACKLOG.md` is the canonical list of open work.
- `docs/plans/initiatives/` holds active multi-step efforts with detailed acceptance criteria.
- `docs/plans/checklists/` holds release, migration, or readiness checklists.
- `docs/plans/notes/` holds supporting notes and reference material, not canonical task state.
- `docs/plans/status/` holds machine-readable state files consumed by tooling.
- `docs/plans/archive/YYYY/` holds completed or superseded plan docs.

## Maintenance rules
- Keep active docs on stable filenames. Reserve date-stamped filenames for archived snapshots.
- Keep each open task in one canonical place in `BACKLOG.md`. Initiative docs may expand on the task but should not carry conflicting status.
- When a plan is completed or superseded, archive it and remove it from the active backlog.
- When a material requirement or architecture decision changes, add or update an ADR and update `docs/decisions/INDEX.md`.
