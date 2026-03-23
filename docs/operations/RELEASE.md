# Release and versioned deployment guide

This project follows SemVer and Keep-a-Changelog.
PromptLock is currently a public OSS prerelease/beta on the path to v1.0.0; this checklist defines the gate for a public OSS prerelease and any later production-targeted release. The GitHub release workflow publishes `v0.x` tags as prereleases/betas and `v1.x` tags as normal releases. `-rc` tags are intentionally unsupported.

## Pre-release checklist

1. Ensure changelog is updated:
   - new work in `## [Unreleased]`
   - release section added as `## [X.Y.Z] - YYYY-MM-DD`
2. Run full validation:

```bash
make release-readiness-gate
```

`make release-readiness-gate` still requires a working Docker daemon because the supported hardened smoke path builds and runs the local `agent-lab` image. The smoke script uses a Go-native PTY helper from `cmd/promptlock-pty-runner` and requires Go plus `jq`. If Docker is unavailable, use the non-Docker core subset as a preflight instead:

```bash
make release-readiness-gate-core
```

`make release-readiness-gate` now runs the supported hardened dual-socket smoke path (`make real-e2e-smoke`) instead of the older dev/insecure compose demo.

3. (Optional re-run) smoke integration in isolation if you want the extra dev/demo compose coverage in addition to the release gate:

```bash
make e2e-compose
```

4. Confirm beta readiness checklist status:
   - `docs/plans/checklists/BETA-READINESS.md`
5. Confirm security policy and disclosure path are published:
   - `SECURITY.md`
6. Confirm non-dev startup prerequisites are set:
   - request/lease state configured through either:
     - `state_store.type=file` with `state_store_file`, or
     - `state_store.type=external` with `state_store.external_url` (`https://...`) + `state_store.external_auth_token_env` env value exported
   - if using `state_store.type=external`, do not treat it as a concurrency-safe multi-node coordination layer; current support is limited to durable external storage integration
   - `auth.store_file` configured
   - auth-store encryption key env exported (`PROMPTLOCK_AUTH_STORE_KEY` or configured `store_encryption_key_env`)
   - `secret_source.type` is not `in_memory`
7. Run storage durability preflight on each target persistence mount:

```bash
make storage-fsync-preflight MOUNT_DIR=/var/lib/promptlock
```

8. Export fsync report attestation key material:

```bash
export PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY='<32+ character HMAC key>'
export PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID='release-key-2026-03'
# optional rotation overlap verification (example):
export PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV='<previous 32+ character HMAC key>'
export PROMPTLOCK_STORAGE_FSYNC_HMAC_KEYRING='release-key-2026-02:PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV'
export PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE='168h'
```

SOPS alternative (decrypted payload must expose the same env names):

```bash
export PROMPTLOCK_SOPS_ENV_FILE=/etc/promptlock/fsync-keys.sops.env
```

9. Run storage fsync release gate for multi-mount persistence paths (generates and validates signed JSON report):

```bash
make storage-fsync-release-gate MOUNT_DIRS=/var/lib/promptlock,/var/log/promptlock FSYNC_REPORT=reports/storage-fsync-report.json
# optional explicit SOPS file override:
# make storage-fsync-release-gate MOUNT_DIRS=/var/lib/promptlock,/var/log/promptlock FSYNC_REPORT=reports/storage-fsync-report.json SOPS_ENV_FILE=/etc/promptlock/fsync-keys.sops.env
```

10. Archive the validated fsync evidence report for release records:
   - `reports/storage-fsync-report.json` (or your configured `FSYNC_REPORT` path)
   - report includes provenance metadata (`schema_version`, `generated_at`, `generated_by`, `hostname`) and attestation fields (`signature.alg`, `signature.key_id`, `signature.value`) validated by gate tooling

## Tag and publish

```bash
git tag -a v0.2.0 -m "PromptLock v0.2.0"
git push origin v0.2.0
```

## Build release artifacts

Run this from a clean checkout that is already at the exact release tag:

```bash
scripts/release-package.sh v0.2.0
```

Produces:
- `dist/promptlock-0.2.0.tar.gz`
- `dist/promptlock-0.2.0.tar.gz.sha256`
- bundled `LICENSE`, `README.md`, and `docs/`
- `RELEASE-METADATA.txt` inside the tarball with exact tag/commit provenance
- release binaries for `promptlock`, `promptlockd`, `promptlock-mcp`, and `promptlock-mcp-launch`

Notes:
- `scripts/release-package.sh` now uses GoReleaser for cross-platform binary builds (`linux/amd64`, `darwin/arm64`).
- `scripts/release-package.sh` refuses to build from a dirty checkout and requires HEAD to be tagged exactly as the requested version, so local release artifacts are created from a clean provenance snapshot.
- `scripts/release-package.sh` now writes embedded provenance metadata and a `sha256` sidecar for the release tarball.
- Tooling is pinned via `go run github.com/goreleaser/goreleaser/v2@v2.7.0` in the script for reproducibility.
- Repository Go and Docker base-image pins are centralized in `.toolchain.env` and enforced by `make toolchain-guard`.
- GitHub Actions, `go.mod`, and Docker build/runtime image tags are aligned to the current `.toolchain.env` values (`GO_VERSION=1.26.1`, `GO_BUILD_IMAGE=golang:1.26.1-alpine3.23`, `RUNTIME_IMAGE=alpine:3.23`).

The GitHub release workflow publishes the tarball, checksum sidecar, and fsync report as release assets for tagged versions. `v0.x` tags are published as prereleases/betas; `v1.x` tags are published as normal releases. `-rc` tags are intentionally unsupported.
Tagged release CI now enforces `make storage-fsync-release-gate`, requires `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY` + `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID`, and still uploads `reports/storage-fsync-report-release-ci.json` as a CI artifact for the workflow run.
Release CI now also exports optional rotation envs when configured (`PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV`, `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEYRING`, `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE`).
For additional rotated keys beyond `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV`, extend workflow `env` with the extra referenced key env names.
If your release flow uses SOPS-managed files instead of pre-exported env vars, ensure `sops` is installed on the runner and set `PROMPTLOCK_SOPS_ENV_FILE`/`SOPS_ENV_FILE` to the decrypted file path during gate execution.
If you intentionally change Go or Docker image versions for release tooling, update `.toolchain.env` first and keep `make toolchain-guard` green before tagging.

## Deployment notes

### Hardened deployment (recommended)
- `security_profile: hardened`
- `auth.enable_auth: true`
- `auth.allow_plaintext_secret_return: false`
- use dual unix sockets for local operator/agent separation
- protect audit log path on host

### Compatibility deployment (temporary)
- `security_profile: dev`
- only for migration/testing scenarios
- document exceptions and timeline for hardening
