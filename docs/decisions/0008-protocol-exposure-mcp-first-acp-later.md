# 0008 - Protocol exposure strategy: MCP first, ACP later

- Status: accepted
- Date: 2026-03-07

## Context
PromptLock should integrate with multiple agent ecosystems. Protocol choice impacts portability and implementation complexity.

## Decision
1. Keep canonical broker API as HTTP/Unix-socket internal contract.
2. Add **MCP adapter first** for broad multi-agent interoperability.
3. Evaluate ACP adapter later if needed for orchestration-specific workflows.

## Rationale
- MCP currently provides broad tool interoperability across agent frameworks.
- ACP can be added later without changing core policy/lease logic.
- Core broker remains protocol-agnostic via ports/adapters architecture.

## Consequences
- Establishes MCP as the first protocol-hardening track for interoperability work.
- Defers ACP-specific complexity until there is a concrete integration need.

## Security implications
- Protocol adapters must not weaken core non-disclosure/lease enforcement.
- Adapter layers must preserve audit events and operator approval semantics.
- MCP exposure must be **capability-first** (execute-with-secret) and not plaintext-secret-first.
- Raw secret-return MCP methods are disallowed by default and require explicit break-glass policy if ever enabled.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
