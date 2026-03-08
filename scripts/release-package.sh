#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  echo "Usage: scripts/release-package.sh <version>" >&2
  exit 1
fi

if [[ ! "$VERSION" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Version must look like v1.2.3 or 1.2.3" >&2
  exit 1
fi

VERSION="${VERSION#v}"
OUT_DIR="dist/promptlock-${VERSION}"
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/bin"

GOOS=linux GOARCH=amd64 go build -o "$OUT_DIR/bin/promptlockd-linux-amd64" ./cmd/promptlockd
GOOS=linux GOARCH=amd64 go build -o "$OUT_DIR/bin/promptlock-linux-amd64" ./cmd/promptlock
GOOS=linux GOARCH=amd64 go build -o "$OUT_DIR/bin/promptlock-mcp-linux-amd64" ./cmd/promptlock-mcp

GOOS=darwin GOARCH=arm64 go build -o "$OUT_DIR/bin/promptlockd-darwin-arm64" ./cmd/promptlockd
GOOS=darwin GOARCH=arm64 go build -o "$OUT_DIR/bin/promptlock-darwin-arm64" ./cmd/promptlock
GOOS=darwin GOARCH=arm64 go build -o "$OUT_DIR/bin/promptlock-mcp-darwin-arm64" ./cmd/promptlock-mcp

cp README.md "$OUT_DIR/"
cp -r docs "$OUT_DIR/docs"

( cd dist && tar -czf "promptlock-${VERSION}.tar.gz" "promptlock-${VERSION}" )

echo "Built dist/promptlock-${VERSION}.tar.gz"
