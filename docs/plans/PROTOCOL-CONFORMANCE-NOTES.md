# MCP protocol conformance notes (initial)

- Status: in-progress

## Current implementation
- stdio JSON-RPC style request/response loop
- methods implemented: initialize, tools/list, tools/call
- capability-first tool only: execute_with_intent

## Known limitations
- limited protocol feature coverage
- no formal conformance suite integration yet
- output mapping is minimal text content structure

## Next conformance tasks
1. Validate against target MCP client(s) used in deployment.
2. Add tests for invalid method shapes and batch/edge-case handling.
3. Add stricter response schema checks.
4. Add compatibility matrix per target MCP runtime.
