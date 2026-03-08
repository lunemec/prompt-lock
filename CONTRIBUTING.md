# Contributing to PromptLock

Thanks for contributing.

## Ground rules

- Security and correctness take priority over feature velocity.
- Follow existing architecture boundaries (hexagonal structure in `docs/architecture/ARCHITECTURE.md`).
- Keep changes small, reviewable, and accompanied by tests.

## Setup

```bash
make ci
```

Before opening a PR, run:

```bash
make validate-final
make security-redteam
```

## Pull request requirements

- Explain the problem and threat/risk impact.
- Include tests for positive and negative paths.
- Update docs when behavior or operator workflow changes.
- Update `CHANGELOG.md` (`[Unreleased]`) for user-visible changes.

## Security-sensitive changes

For auth, execution policy, secret handling, transport, or audit changes:

- add/extend unit tests in affected package,
- include at least one adversarial/abuse test case,
- avoid introducing new plaintext secret surfaces,
- document migration or rollout implications.

## Security reporting

Do not file public issues for vulnerabilities. See `SECURITY.md`.

## Style and architecture

- Prefer transport-thin handlers and policy/business logic in app/core layers.
- Keep dependency direction inward-only.
- Avoid adding direct coupling between unrelated adapters.
- Keep contributor tooling aligned with the primary project stack (Go). Adding new runtime/toolchain dependencies requires explicit maintainer approval and documented justification.

## Commit guidance

- Use clear commit messages with scope (`security:`, `docs:`, `refactor:`).
- Keep one logical concern per commit where practical.
