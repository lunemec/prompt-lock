#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

require_clean_worktree() {
  if ! git -C "$ROOT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "release packaging requires a git checkout so provenance can be verified" >&2
    exit 1
  fi

  local dirty_status
  dirty_status="$(git -C "$ROOT_DIR" status --porcelain=2 --untracked-files=all --ignore-submodules=all)"
  if [[ -n "$dirty_status" ]]; then
    echo "release packaging requires a clean git checkout; refusing to build from a dirty tree" >&2
    printf '%s\n' "$dirty_status" | sed 's/^/  /' >&2
    exit 1
  fi
}

require_tagged_release() {
  local tag="$1"
  local exact_tag

  exact_tag="$(git -C "$ROOT_DIR" describe --tags --exact-match --match "$tag" 2>/dev/null || true)"
  if [[ "$exact_tag" != "$tag" ]]; then
    echo "release packaging requires HEAD to be tagged exactly as $tag" >&2
    exit 1
  fi
}

sha256_file() {
  local file="$1"
  local out="$2"

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" > "$out"
    return
  fi

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" > "$out"
    return
  fi

  echo "release packaging requires sha256sum or shasum" >&2
  exit 1
}

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
TAG="v${VERSION}"
OUT_DIR="dist/promptlock-${VERSION}"
GORELEASER_DIST=".goreleaser-dist"
GORELEASER_VERSION="${GORELEASER_VERSION:-v2.7.0}"

cd "$ROOT_DIR"
require_clean_worktree
require_tagged_release "$TAG"

rm -rf "$OUT_DIR" "$GORELEASER_DIST"
mkdir -p "$OUT_DIR/bin"

SOURCE_COMMIT="$(git -C "$ROOT_DIR" rev-parse HEAD)"
cat > "$OUT_DIR/RELEASE-METADATA.txt" <<EOF
PromptLock release provenance
version: ${VERSION}
tag: ${TAG}
commit: ${SOURCE_COMMIT}
EOF

go run "github.com/goreleaser/goreleaser/v2@${GORELEASER_VERSION}" build --snapshot --clean --config .goreleaser.yaml

for bin in \
  promptlockd-linux-amd64 \
  promptlock-linux-amd64 \
  promptlock-mcp-linux-amd64 \
  promptlock-mcp-launch-linux-amd64 \
  promptlockd-darwin-arm64 \
  promptlock-darwin-arm64 \
  promptlock-mcp-darwin-arm64 \
  promptlock-mcp-launch-darwin-arm64
do
  src="${GORELEASER_DIST}/${bin}"
  if [[ ! -f "$src" ]]; then
    echo "missing expected GoReleaser artifact: $src" >&2
    exit 1
  fi
  cp "$src" "$OUT_DIR/bin/$bin"
done

cp LICENSE README.md "$OUT_DIR/"
cp -r docs "$OUT_DIR/docs"

( cd dist && tar -czf "promptlock-${VERSION}.tar.gz" "promptlock-${VERSION}" )
sha256_file "dist/promptlock-${VERSION}.tar.gz" "dist/promptlock-${VERSION}.tar.gz.sha256"
rm -rf "$GORELEASER_DIST"

echo "Built dist/promptlock-${VERSION}.tar.gz"
