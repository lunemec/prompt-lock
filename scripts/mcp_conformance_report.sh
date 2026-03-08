#!/usr/bin/env bash
set -euo pipefail

mkdir -p reports

go test -json ./cmd/promptlock-mcp > reports/mcp-conformance.json

echo "MCP conformance report written to reports/mcp-conformance.json"
