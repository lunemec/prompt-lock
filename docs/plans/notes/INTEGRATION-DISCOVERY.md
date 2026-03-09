# Integration discovery plan

This is a reference note, not a canonical task-status file. Use `docs/plans/BACKLOG.md` for current status.

- Goal: confirm practical auth injection paths for Codex + additional CLIs.

## Work items
1. Codex auth path verification
   - config options
   - env var mode vs auth file mode
   - renewal behavior expectations
2. Claude/Gemini/OpenClaw auth path notes
3. Define first adapter interface in Go
4. Define test harness for lease expiry + renewal

## Deliverables
- updated compatibility matrix
- concrete adapter backlog with priority
- MVP integration test script(s)
