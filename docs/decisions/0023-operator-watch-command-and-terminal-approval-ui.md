# 0023 - Operator watch command and minimal terminal approval UI

- Status: accepted
- Date: 2026-03-14

## Context
PromptLock's host-side approval command was named `approve-queue`, but the primary operator workflow is long-running queue watching rather than one-shot approval. The old prompt loop also appended repeated output over time, which made it hard to distinguish fresh requests from retried or skipped ones during active approvals.

## Decision
1. Rename the operator-facing command from `promptlock approve-queue` to `promptlock watch`.
2. Keep the operator surface stdlib-only and implement a minimal terminal UI rather than adding a third-party TUI dependency.
3. In interactive terminal sessions, clear and redraw the screen when the visible queue state changes or after approve/deny actions.
4. Keep one-shot operator actions under the same namespace:
   - `promptlock watch list`
   - `promptlock watch allow <request_id>`
   - `promptlock watch deny <request_id>`
5. Require explicit operator input:
   - `y`/`yes` to approve
   - `n`/`no` to deny
   - `s`/`skip` to suppress the current request until queue membership changes
   - `q`/`quit` to exit without mutating requests

## Consequences
- The operator workflow now matches the command name: `watch` clearly implies a long-running queue monitor.
- Interactive use is less noisy because queue redraws replace appended blocks of stale output.
- The rename is intentionally breaking; existing operator scripts using `approve-queue` must be updated.
- Non-TTY execution still works, but falls back to plain output instead of full-screen redraw behavior.

## Security implications
- The new UI reduces accidental operator mistakes caused by stale repeated prompts or implicit skip behavior.
- This decision changes presentation and ergonomics only; it does not alter broker authorization or lease enforcement boundaries.
- The watch UI still depends on explicit operator token handling and should be run only from trusted host environments.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
