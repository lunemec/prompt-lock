# Planning Docs

This directory is the repository's execution-state surface.

## Canonical files
- `ACTIVE-PLAN.md` holds the current handoff: present focus, active review items, and validation expectations.
- `BACKLOG.md` holds the canonical list of open work.

## Subdirectories
- `initiatives/` holds active multi-step work.
- `checklists/` holds release, migration, and readiness gates.
- `notes/` holds supporting reference material only.
- `status/` holds machine-readable state consumed by tooling.
- `archive/YYYY/` holds completed or superseded plan/history docs.

## Required workflow
1. Update `ACTIVE-PLAN.md` when focus, active risks, or delivery gates change.
2. Update `BACKLOG.md` whenever open work is added, reprioritized, blocked, or closed.
3. Put detailed live scope in an initiative or checklist, not in `ACTIVE-PLAN.md`.
4. Archive completed or superseded planning docs instead of leaving them active.

## Rules
- `ACTIVE-PLAN.md` is a handoff file, not a changelog.
- Keep open work in one canonical place.
- Notes may explain context, but they are never the source of truth for status.
- Use stable filenames for active docs and date-stamped names only in archives.
