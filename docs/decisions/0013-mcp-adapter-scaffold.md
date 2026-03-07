# 0013 - MCP adapter scaffold (capability-first)

- Status: accepted
- Date: 2026-03-07

## Context
MCP support was requested after top security risks were addressed with broker-exec and policy controls.

## Decision
- Add an experimental MCP stdio adapter exposing capability-first tooling only.
- Initial MCP tool: `execute_with_intent`.
- Do not expose plaintext secret retrieval via MCP.

## Consequences
- Enables interoperability experiments with MCP-compatible agents.
- Requires iterative hardening and protocol conformance testing before production use.

## Security implications
- Maintains non-disclosure direction by avoiding raw secret-return MCP tools.
- Risk remains if executed commands intentionally exfiltrate data; mitigated by broker execution policy and audits.
