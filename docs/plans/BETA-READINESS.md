# Beta Readiness Checklist (PromptLock)

- Status: in-progress

## Security foundations
- [x] Role-separated auth model (operator vs agent session)
- [x] Pairing/session foundation with idle + absolute grant limits
- [x] Transport hardening defaults + unix socket support
- [x] Audit actor attribution
- [x] Tamper-evident audit hash chain
- [x] Plaintext secret-return policy gate
- [x] Broker-exec secure path with execution policy controls

## Testing depth
- [x] Unit tests for core policy/auth flows
- [x] Fuzz tests for MCP arg parsing and execute command policy
- [x] MCP stdio harness (initialize/tools/list)
- [x] MCP tools/call mocked-broker roundtrip
- [x] MCP tools/call live broker-backed positive path
- [x] MCP negative tests: denied, timeout, missing/invalid session, policy denied

## Remaining before beta tag
- [ ] Add stricter MCP protocol conformance tests vs target MCP clients
- [x] Add documented threat-model walkthrough with sample attack simulations
- [x] Add operational runbook for key rotation/revocation drills
- [ ] Add release packaging + versioned deployment guide
