# AGENTS.md

Purpose: repository map and hard rules for agents working in PromptLock.

## Required read order
1. `README.md`
2. `docs/README.md`
3. `docs/architecture/ARCHITECTURE.md`
4. `docs/standards/ENGINEERING-STANDARDS.md`
5. `docs/plans/ACTIVE-PLAN.md`
6. `docs/plans/BACKLOG.md`
7. The specific ADR, initiative, checklist, or ops doc for the task at hand

## Clone-and-try runtime rules (wrapper container agents)
- Scope: applies to coding agents running inside a container launched via `promptlock auth docker-run` for this repo.
- Use PromptLock MCP when running tests or when commands need secrets/env vars; do not bypass approval by reading secret files or raw env directly.
- Default MCP tool flow for this repo:
  - tool: `execute_with_intent`
  - default quickstart intent: `run_tests`
  - demo env-path: `env_path: "demo-envs/github.env"` (disposable demo value only)
- Prefer direct argv commands in MCP calls (for example `["make","demo-run-env-showcase-tests"]`) instead of shell wrappers.
- Before first MCP request in the container, run `promptlock mcp doctor`; if needed, register with `codex mcp add promptlock -- promptlock-mcp-launch`.
- If MCP is unavailable or a request is denied, stop and report the blocker rather than falling back to direct secret access.

## Non-negotiable engineering rules
- Hexagonal architecture is mandatory: domain/core stays isolated from transport and adapters.
- Security-sensitive work follows Red-Green-Blue TDD:
  - Red: add a failing test first
  - Green: add the minimum implementation to pass
  - Blue: refactor and harden while keeping tests green
- Maximum practical test coverage is mandatory for security-critical paths.
- Any security concern, abuse path, or trust-boundary ambiguity must be called out explicitly in plan updates and task completion.
- Expose developer and operator workflows through `Makefile` targets.
- Keep new tooling and automation in Go unless a secondary runtime is explicitly approved.
- Config names, schema names, and docs must match enforced runtime behavior. If a legacy name is kept for compatibility, document the mismatch where operators will see it and track cleanup in `docs/plans/BACKLOG.md`.
- Do not overstate security posture. Basename-only executable matching is not provenance, and unsupported deployment paths must not be described as supported.
- Significant requirement or architecture changes require an ADR in `docs/decisions/` and a matching `docs/decisions/INDEX.md` update.
- Keep `CHANGELOG.md` in Keep-a-Changelog format and record new repo changes under `[Unreleased]`.
- `make validate-final` is the required final gate before commit.

## Code hygiene rules
- Extract by responsibility, not by line count alone.
- Keep command packages focused on CLI/bootstrap concerns; do not let `cmd/*` files become mixed transport, policy, and workflow hubs.
- Keep large policy data tables or parsing helpers separate from orchestration logic when that improves readability without changing behavior.
- Avoid “active docs as changelog” drift: `ACTIVE-PLAN.md` is a handoff file, not the full historical record.
- Prefer reducing duplication and ownership ambiguity over cosmetic churn.

## Documentation rules
- `docs/README.md` is the documentation map and must match the actual tree.
- `docs/plans/ACTIVE-PLAN.md` is the canonical handoff file.
- `docs/plans/BACKLOG.md` is the canonical list of open work.
- `docs/plans/initiatives/` is for active multi-step efforts.
- `docs/plans/checklists/` is for release and migration gates.
- `docs/plans/notes/` is for supporting reference material, not canonical status.
- `docs/plans/status/` is for machine-readable state files.
- `docs/plans/archive/YYYY/` is for completed or superseded plan/history docs.
- Keep open work in one canonical place. Do not duplicate live status across active files.
- When a plan or initiative is completed, archive it and update the active planning surface in the same change.

## Onboarding and UX review rubric
- Treat README, setup output, and CLI `--help` text as one user journey. If one changes, inspect the others for drift.
- Optimize first for first-command discoverability, copy-pasteability, and host-versus-container execution clarity.
- Keep demo-only shortcuts clearly marked. Do not let dev/demo flows read like the supported hardened path.
- Spell out where commands run and which socket/token they expect when that is not obvious from the command itself.
- For agent-facing UX/doc changes, prefer short mental-model text near the command over routing the reader through multiple deep docs before first success.

## Validation expectations for docs or CLI UX changes
- Run the narrowest relevant command tests first (for example `go test ./cmd/promptlock` for CLI/help/setup changes).
- Manually verify every README command you edit still matches the current CLI surface.
- Run `make docs` when documentation structure or entrypoints change.
- Run `make validate-final` before finishing the task unless the user explicitly scopes work to planning/review only or the environment prevents it.

## Review expectations
- Default to a strict code review mindset: correctness, security, behavior drift, architectural boundary violations, clutter, and test gaps.
- For cleanliness/refactor work, preserve behavior unless an existing test or documented mismatch already proves the current behavior is wrong.
- If the worktree is dirty, do not revert unrelated changes. Read them carefully and work around them.

## Completion output
- Summary of changes
- Tests run and results
- Security findings and concerns
- Files changed
- Follow-up actions

## Quick map
- Docs map: `docs/README.md`
- Architecture: `docs/architecture/`
- Plans: `docs/plans/`
- ADR index: `docs/decisions/INDEX.md`
- Standards: `docs/standards/`
- Operations: `docs/operations/`
- Context: `docs/context/`
