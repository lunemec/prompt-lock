# BACKLOG

Updated: 2026-03-16

This is the canonical list of open work. Initiative docs may hold detail, but status should stay aligned here. Completed work belongs in `ACTIVE-PLAN.md`, initiative/checklist docs, or archived plan history rather than this file.

## Open items
- Implement ADR `0030` CLI unification:
  - add `promptlock daemon <start|stop|status>` lifecycle subcommands,
  - add watch convenience startup flow (`watch --spawn-daemon` or final equivalent),
  - keep `promptlockd` transition compatibility until migration docs/tests are complete.
  - tracking note: `docs/plans/notes/CLI-UNIFICATION-PLAN.md`
