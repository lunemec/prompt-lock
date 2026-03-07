# ENGINEERING STANDARDS

## Non-negotiables
- Hexagonal architecture enforced by code layout and tests.
- Maximum practical test coverage for domain + policy + security-sensitive adapters.
- Red-Green-Blue TDD required for all feature work.

## Red-Green-Blue definition
1. **Red**: add failing test proving required behavior/security rule.
2. **Green**: minimal implementation to pass.
3. **Blue**: refactor + harden security posture while keeping tests green.

## Security reporting requirement
Every task completion must include:
- discovered security risks
- potential abuse/exfiltration paths
- mitigation status (implemented / deferred)

If unsure about security impact, explicitly escalate in output.

## Project process requirements
- Provide Makefile commands for common contributor workflows.
- Document architectural and requirement decisions in ADRs.
- Maintain CHANGELOG.md using Keep-a-Changelog.
- New changes must be under `[Unreleased]`.
- Released versions must use proper SemVer headings.
- `make validate-final` must pass before commit/merge.
