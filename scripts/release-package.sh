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
GORELEASER_DIST=".goreleaser-dist"
GORELEASER_VERSION="${GORELEASER_VERSION:-v2.7.0}"

rm -rf "$OUT_DIR" "$GORELEASER_DIST"
mkdir -p "$OUT_DIR/bin"

go run "github.com/goreleaser/goreleaser/v2@${GORELEASER_VERSION}" build --snapshot --clean --config .goreleaser.yaml

for bin in \
  promptlockd-linux-amd64 \
  promptlock-linux-amd64 \
  promptlock-mcp-linux-amd64 \
  promptlockd-darwin-arm64 \
  promptlock-darwin-arm64 \
  promptlock-mcp-darwin-arm64
do
  src="${GORELEASER_DIST}/${bin}"
  if [[ ! -f "$src" ]]; then
    echo "missing expected GoReleaser artifact: $src" >&2
    exit 1
  fi
  cp "$src" "$OUT_DIR/bin/$bin"
done

cp README.md "$OUT_DIR/"
cp -r docs "$OUT_DIR/docs"

( cd dist && tar -czf "promptlock-${VERSION}.tar.gz" "promptlock-${VERSION}" )
rm -rf "$GORELEASER_DIST"

echo "Built dist/promptlock-${VERSION}.tar.gz"
