# MCP protocol conformance notes (initial)

This is a reference note, not a canonical task-status file. Use `docs/plans/BACKLOG.md` for current status.

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
- output mapping is minimal text content structure

## Protocol behavior currently enforced
- batch requests are explicitly rejected (`-32600`)
- invalid request shape/version is rejected (`-32600`)
- response schema tests assert stable JSON-RPC envelope (`jsonrpc`, `id`, exactly one of `result/error`)

## Hardening implemented
- stdio scanner input line cap (1 MiB) to reduce oversized input abuse risk
- parser and argument validation negative-path tests

## Next conformance tasks
1. Validate against target MCP client(s) used in deployment.
2. Add tests for invalid method shapes and batch/edge-case handling.
3. Add stricter response schema checks.
4. Add compatibility matrix per target MCP runtime.
