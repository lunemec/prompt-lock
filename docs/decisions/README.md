# Architecture Decision Records (ADR)

All material technical and security decisions must be documented as ADRs.

## Requirements
- If requirements change, record the change and rationale in a new ADR.
- Never silently replace prior requirements; supersede them explicitly.
- Link ADRs from relevant docs/plans/PR descriptions.
- Update `INDEX.md` in the same change whenever you add or modify an ADR.

## Naming
Use sequential files:
- `0001-title.md`
- `0002-title.md`

## Minimal ADR fields
- Status (proposed/accepted/superseded)
- Context
- Decision
- Consequences
- Security implications
- Supersedes / Superseded by (if applicable)

## Conventions
- Use lowercase status values for new or modified ADRs: `proposed`, `accepted`, `superseded`.
- `INDEX.md` is the decision log entrypoint for agents and maintainers.
- Legacy ADRs may retain older formatting until they are touched, but any edited ADR should be brought in line with these rules.
