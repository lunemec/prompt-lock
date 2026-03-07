# 0001 - Project governance requirements

- Status: accepted
- Date: 2026-03-07

## Context
This project is security-sensitive. Agent and human contributors need clear quality/security governance.

## Decision
We require:
1. Hexagonal architecture.
2. Maximum practical test coverage.
3. Red-Green-Blue TDD.
4. Explicit security issue reporting.
5. Keep-a-Changelog with mandatory [Unreleased] section.
6. Final validation gate before commit including changelog validation.
7. Host-side tamper-evident audit trail as critical requirement.

## Consequences
- Development speed may be slower initially but with stronger security controls and traceability.
- CI and local validation scripts are mandatory.

## Security implications
- Reduces risk of undocumented security regressions.
- Improves forensic capability and governance under incidents.
