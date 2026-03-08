# MCP protocol conformance notes (initial)

- Status: in-progress

## Current implementation
- stdio JSON-RPC style request/response loop
- methods implemented: initialize, tools/list, tools/call
- capability-first tool only: execute_with_intent
- harness coverage includes:
  - initialize/tools/list roundtrip
  - tools/call mocked and live broker-backed positive path
  - negative paths: malformed JSON, unknown tool, validation error, denied, timeout, missing/invalid session

## Known limitations
- no formal external MCP conformance suite integration yet
- batch request handling not implemented
- output mapping is minimal text content structure

## Next conformance tasks
1. Validate against target MCP client(s) used in deployment.
2. Add tests for invalid method shapes and batch/edge-case handling.
3. Add stricter response schema checks.
4. Add compatibility matrix per target MCP runtime.
