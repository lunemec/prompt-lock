# MCP integration tests roadmap

- Status: in-progress

## Goal
Move MCP adapter from scaffold-level confidence to integration-level confidence with real stdio roundtrips.

## Test stages
1. Unit tests (current)
   - argument validation
   - response/error shape sanity
2. Local protocol harness
   - spawn `promptlock-mcp`
   - send initialize/tools/list/tools/call lines over stdin
   - assert JSON-RPC outputs over stdout
   - status: implemented for initialize/tools/list and tools/call with mocked broker roundtrip
3. End-to-end broker-backed test
   - start test broker with auth/session fixture
   - run MCP execute_with_intent flow
   - assert lease lifecycle + execute output + audit events
   - status: partially implemented (live broker-backed positive path added)
4. Negative/security tests
   - malformed JSON-RPC payloads
   - oversized arguments
   - denied intents and timeout paths
   - status: partially implemented (denied path, timeout path, missing-session-token path)

## Exit criteria for "MCP-ready beta"
- deterministic harness test suite in CI
- end-to-end success + failure paths covered
- documented known limitations
