#!/usr/bin/env bash
set -euo pipefail

# Thin wrapper around the Go security scanner so the shell guard cannot drift
# from the repo's canonical scan rules.

root="$(git rev-parse --show-toplevel)"
cd "$root"
go run ./cmd/promptlock-validate-security
