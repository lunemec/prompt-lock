# Planning Docs

This directory is the repository's execution-state surface. Agents must keep it structured and current.

## Canonical files
- `ACTIVE-PLAN.md` — current handoff, focus, recent completions, and validation expectations
- `BACKLOG.md` — canonical open task list with priorities and references

## Subdirectories
- `initiatives/` — active multi-step workstreams with detailed scope and acceptance criteria
- `checklists/` — release, migration, and readiness checklists
- `notes/` — supporting notes, investigation records, and non-canonical working material
- `status/` — machine-readable files used by tooling and CI gates
- `archive/YYYY/` — completed or superseded plan docs retained for history

## Required workflow
1. Update `ACTIVE-PLAN.md` when the current focus, recently completed work, or validation expectations change.
2. Update `BACKLOG.md` whenever open work is added, reprioritized, blocked, or completed.
3. Put detail in the relevant initiative or checklist, not in `ACTIVE-PLAN.md`.
4. Archive completed or superseded plan docs instead of leaving them in the active root.

## Naming and status rules
- Use stable names for active docs.
- Use date-stamped names only for archive snapshots and historical records.
- Keep canonical status in `BACKLOG.md`; do not duplicate active status tracking across multiple files.
- Notes may describe implementation progress or coverage, but they must not be treated as the source of truth for open-task status.
