# Release and versioned deployment guide

This project follows SemVer and Keep-a-Changelog.
PromptLock is currently experimental; this checklist defines the production-readiness release gate.

## Pre-release checklist

1. Ensure changelog is updated:
   - new work in `## [Unreleased]`
   - release section added as `## [X.Y.Z] - YYYY-MM-DD`
2. Run full validation:

```bash
make validate-final
make production-readiness-gate
make fuzz
```

3. Run smoke integration:

```bash
make e2e-compose
```

4. Confirm beta readiness checklist status:
   - `docs/plans/checklists/BETA-READINESS.md`
5. Confirm security policy and disclosure path are published:
   - `SECURITY.md`
6. Confirm non-dev startup prerequisites are set:
   - `state_store_file` configured
   - `auth.store_file` configured
   - auth-store encryption key env exported (`PROMPTLOCK_AUTH_STORE_KEY` or configured `store_encryption_key_env`)
   - `secret_source.type` is not `in_memory`
7. Run storage durability preflight on each target persistence mount:

```bash
make storage-fsync-preflight MOUNT_DIR=/var/lib/promptlock
```

8. Run storage fsync release gate for multi-mount persistence paths (generates and validates JSON report):

```bash
make storage-fsync-release-gate MOUNT_DIRS=/var/lib/promptlock,/var/log/promptlock FSYNC_REPORT=reports/storage-fsync-report.json
```

9. Archive the validated fsync evidence report for release records:
   - `reports/storage-fsync-report.json` (or your configured `FSYNC_REPORT` path)
   - report includes provenance metadata (`schema_version`, `generated_at`, `generated_by`, `hostname`) validated by gate tooling

## Build release artifacts

```bash
scripts/release-package.sh v0.2.0
```

Produces:
- `dist/promptlock-0.2.0.tar.gz`

Notes:
- `scripts/release-package.sh` now uses GoReleaser for cross-platform binary builds (`linux/amd64`, `darwin/arm64`).
- Tooling is pinned via `go run github.com/goreleaser/goreleaser/v2@v2.7.0` in the script for reproducibility and Go 1.23 compatibility.

## Tag and publish

```bash
git tag -a v0.2.0 -m "PromptLock v0.2.0"
git push origin v0.2.0
```

GitHub release workflow should attach build artifacts for tagged versions.
Tagged release CI now enforces `make storage-fsync-release-gate` and uploads `reports/storage-fsync-report-release-ci.json` as a release workflow artifact.

## Deployment notes

### Hardened deployment (recommended)
- `security_profile: hardened`
- `auth.enable_auth: true`
- `auth.allow_plaintext_secret_return: false`
- prefer `unix_socket` transport
- protect audit log path on host

### Compatibility deployment (temporary)
- `security_profile: dev`
- only for migration/testing scenarios
- document exceptions and timeline for hardening
