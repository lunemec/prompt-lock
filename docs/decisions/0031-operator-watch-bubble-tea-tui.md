# 0031 - Operator watch Bubble Tea TUI with plain fallback

- Status: accepted
- Date: 2026-03-22

## Context
ADR 0023 established the `promptlock watch` operator flow and required a stdlib-only terminal UI. That kept the initial implementation small, but the resulting redraw loop left the command bootstrap, queue polling, rendering, and prompt handling concentrated in one file and limited the operator context visible during active approvals.

PromptLock now needs a clearer interactive watch experience without changing broker authorization, request schemas, or the one-shot `watch list|allow|deny` surfaces.

## Decision
1. Keep the `promptlock watch` command surface, flags, and approval semantics unchanged.
2. Use Bubble Tea and Lip Gloss for TTY `promptlock watch` sessions.
3. Keep a plain prompt/output fallback for non-TTY, redirected, and `--once` execution so existing automation remains compatible.
4. Keep broker polling and approve/deny transport behind the existing CLI-side watch client boundary rather than moving transport or policy logic into the UI model.
5. Preserve the existing operator keybindings:
   - `y` approve
   - `n` deny
   - `s` skip
   - `q` quit

## Consequences
- Interactive operators get a stable header/footer layout with queue context and last-action status while polling continues in the background.
- The command layer stays focused on flag parsing, daemon bootstrap, broker resolution, and env-path preflight; the TUI lives in dedicated watch UI files.
- PromptLock takes on a small Go-module dependency for the interactive watch experience.
- Non-interactive scripts and tests keep the plain fallback path instead of inheriting terminal-specific behavior.

## Security implications
- This ADR changes operator presentation only; broker-side authorization, lease issuance, env-path checks, and audit boundaries stay unchanged.
- The plain fallback remains available so automation does not need to emulate a terminal to approve or inspect requests.
- Env-path preflight still runs before either watch UI path starts, preserving the fail-fast operator check for approved `.env` flows.

## Supersedes / Superseded by
- Supersedes: 0023
- Superseded by: none
