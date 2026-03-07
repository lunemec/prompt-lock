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

## Security implications
- Protocol adapters must not weaken core non-disclosure/lease enforcement.
- Adapter layers must preserve audit events and operator approval semantics.
