# MCP adapter (experimental)

PromptLock now includes an experimental MCP stdio adapter:

- binary path: `cmd/promptlock-mcp`
- tool exposed: `execute_with_intent`

## Security model
- capability-first flow only (no plaintext secret fetch tool)
- uses broker lease + execute endpoints under the hood
- requires `PROMPTLOCK_SESSION_TOKEN` for agent auth

## Notes
- This is an early adapter scaffold for interoperability testing.
- Keep hardened broker config enabled (`allow_plaintext_secret_return=false` and broker-exec path).
- Adapter now includes baseline input validation for intent, command, and TTL bounds.
- Production hardening for MCP should still include deeper protocol-conformance tests against target MCP clients.
