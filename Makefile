.PHONY: help lint toolchain-guard vet vulncheck race test fuzz security security-redteam security-redteam-live security-redteam-live-hardened hardened-smoke real-e2e-smoke mcp-conformance-report production-readiness-gate release-readiness-gate-core release-readiness-gate leak-guard storage-fsync-preflight storage-fsync-report storage-fsync-validate storage-fsync-release-gate ci-redteam-full arch-conformance docs validate-changelog hygiene validate-final ci e2e-compose setup-local-docker build-agent-lab demo-print-github-token demo-run-env-showcase-tests release-package

FSYNC_REPORT ?= reports/storage-fsync-report.json
FSYNC_HMAC_KEY_ENV ?= PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY
FSYNC_HMAC_KEY_ID_ENV ?= PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID
FSYNC_HMAC_KEYRING_ENV ?= PROMPTLOCK_STORAGE_FSYNC_HMAC_KEYRING
FSYNC_HMAC_KEY_OVERLAP_MAX_AGE_ENV ?= PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE
SOPS_ENV_FILE ?=
SHIPPED_SHELL_WORKFLOWS := $(sort $(wildcard scripts/*.sh))

help:
	@echo "Targets: lint toolchain-guard vet vulncheck race test fuzz security security-redteam security-redteam-live security-redteam-live-hardened hardened-smoke real-e2e-smoke mcp-conformance-report production-readiness-gate release-readiness-gate-core release-readiness-gate leak-guard storage-fsync-preflight storage-fsync-report storage-fsync-validate storage-fsync-release-gate ci-redteam-full arch-conformance docs validate-changelog validate-final ci e2e-compose setup-local-docker build-agent-lab demo-print-github-token demo-run-env-showcase-tests release-package"

lint:
	bash -n $(SHIPPED_SHELL_WORKFLOWS)

toolchain-guard:
	go mod verify
	go run ./cmd/promptlock-validate-toolchain

vet:
	go vet ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...

race:
	go test -race ./cmd/promptlock ./cmd/promptlockd ./internal/app ./internal/adapters/envpath

test:
	go test ./...

fuzz:
	go test ./cmd/promptlock-mcp -run=^$$ -fuzz=FuzzParseAndValidateExecArgs -fuzztime=5s
	go test ./cmd/promptlockd -run=^$$ -fuzz=FuzzValidateExecuteCommand -fuzztime=5s

security:
	go run ./cmd/promptlock-validate-security

security-redteam:
	bash scripts/run_redteam_e2e.sh

security-redteam-live:
	mkdir -p reports
	go run ./cmd/promptlock-redteam-live reports/redteam-live.json dev

security-redteam-live-hardened:
	mkdir -p reports
	go run ./cmd/promptlock-redteam-live reports/redteam-live-hardened.json hardened

hardened-smoke:
	bash scripts/run_hardened_smoke.sh

real-e2e-smoke:
	bash scripts/run_real_e2e_smoke.sh

mcp-conformance-report:
	bash scripts/mcp_conformance_report.sh

production-readiness-gate:
	go run ./cmd/promptlock-readiness-check --file docs/plans/status/PRODUCTION-READINESS-STATUS.json --require-release-gating

release-readiness-gate-core:
	$(MAKE) validate-final
	$(MAKE) vulncheck
	$(MAKE) production-readiness-gate
	$(MAKE) fuzz

release-readiness-gate: release-readiness-gate-core
	$(MAKE) real-e2e-smoke

leak-guard:
	bash scripts/validate_no_secret_leaks.sh

storage-fsync-preflight:
	@test -n "$(MOUNT_DIR)" || (echo "Usage: make storage-fsync-preflight MOUNT_DIR=/path/to/mount" && exit 1)
	go run ./cmd/promptlock-storage-fsync-check --dir "$(MOUNT_DIR)"

storage-fsync-report:
	@test -n "$(MOUNT_DIRS)" || (echo "Usage: make storage-fsync-report MOUNT_DIRS=/path/a,/path/b [FSYNC_REPORT=reports/storage-fsync-report.json] [SOPS_ENV_FILE=/path/to/keys.sops.env]" && exit 1)
	@if [ -z "$(SOPS_ENV_FILE)" ] && [ -z "$$PROMPTLOCK_SOPS_ENV_FILE" ]; then \
		test -n "$$(printenv $(FSYNC_HMAC_KEY_ENV))" || (echo "Missing env: $(FSYNC_HMAC_KEY_ENV)" && exit 1); \
		test -n "$$(printenv $(FSYNC_HMAC_KEY_ID_ENV))" || (echo "Missing env: $(FSYNC_HMAC_KEY_ID_ENV)" && exit 1); \
	fi
	@mkdir -p "$(dir $(FSYNC_REPORT))"
	go run ./cmd/promptlock-storage-fsync-check --dir-list "$(MOUNT_DIRS)" --json --sops-env-file "$(SOPS_ENV_FILE)" --hmac-key-env "$(FSYNC_HMAC_KEY_ENV)" --hmac-key-id-env "$(FSYNC_HMAC_KEY_ID_ENV)" > "$(FSYNC_REPORT)"
	@echo "storage fsync report written to $(FSYNC_REPORT)"

storage-fsync-validate:
	@test -n "$(FSYNC_REPORT)" || (echo "Usage: make storage-fsync-validate FSYNC_REPORT=reports/storage-fsync-report.json" && exit 1)
	@test -f "$(FSYNC_REPORT)" || (echo "Missing fsync report file: $(FSYNC_REPORT)" && exit 1)
	@if [ -z "$(SOPS_ENV_FILE)" ] && [ -z "$$PROMPTLOCK_SOPS_ENV_FILE" ]; then \
		test -n "$$(printenv $(FSYNC_HMAC_KEY_ENV))" || (echo "Missing env: $(FSYNC_HMAC_KEY_ENV)" && exit 1); \
		test -n "$$(printenv $(FSYNC_HMAC_KEY_ID_ENV))" || (echo "Missing env: $(FSYNC_HMAC_KEY_ID_ENV)" && exit 1); \
	fi
	go run ./cmd/promptlock-storage-fsync-validate --file "$(FSYNC_REPORT)" --sops-env-file "$(SOPS_ENV_FILE)" --hmac-key-env "$(FSYNC_HMAC_KEY_ENV)" --hmac-key-id-env "$(FSYNC_HMAC_KEY_ID_ENV)" --hmac-keyring-env "$(FSYNC_HMAC_KEYRING_ENV)" --hmac-key-overlap-max-age-env "$(FSYNC_HMAC_KEY_OVERLAP_MAX_AGE_ENV)"

storage-fsync-release-gate:
	@test -n "$(MOUNT_DIRS)" || (echo "Usage: make storage-fsync-release-gate MOUNT_DIRS=/path/a,/path/b [FSYNC_REPORT=reports/storage-fsync-report.json] [SOPS_ENV_FILE=/path/to/keys.sops.env]" && exit 1)
	@if [ -z "$(SOPS_ENV_FILE)" ] && [ -z "$$PROMPTLOCK_SOPS_ENV_FILE" ]; then \
		test -n "$$(printenv $(FSYNC_HMAC_KEY_ENV))" || (echo "Missing env: $(FSYNC_HMAC_KEY_ENV)" && exit 1); \
		test -n "$$(printenv $(FSYNC_HMAC_KEY_ID_ENV))" || (echo "Missing env: $(FSYNC_HMAC_KEY_ID_ENV)" && exit 1); \
	fi
	@mkdir -p "$(dir $(FSYNC_REPORT))"
	go run ./cmd/promptlock-storage-fsync-check --dir-list "$(MOUNT_DIRS)" --json --sops-env-file "$(SOPS_ENV_FILE)" --hmac-key-env "$(FSYNC_HMAC_KEY_ENV)" --hmac-key-id-env "$(FSYNC_HMAC_KEY_ID_ENV)" > "$(FSYNC_REPORT)"
	go run ./cmd/promptlock-storage-fsync-validate --file "$(FSYNC_REPORT)" --sops-env-file "$(SOPS_ENV_FILE)" --hmac-key-env "$(FSYNC_HMAC_KEY_ENV)" --hmac-key-id-env "$(FSYNC_HMAC_KEY_ID_ENV)" --hmac-keyring-env "$(FSYNC_HMAC_KEYRING_ENV)" --hmac-key-overlap-max-age-env "$(FSYNC_HMAC_KEY_OVERLAP_MAX_AGE_ENV)"
	@echo "storage fsync release gate passed with report $(FSYNC_REPORT)"

ci-redteam-full: validate-final security-redteam-live security-redteam-live-hardened mcp-conformance-report real-e2e-smoke leak-guard
	@echo "Full red-team CI profile passed."

arch-conformance:
	bash scripts/verify_architecture_conformance.sh
	go test ./cmd/promptlock -run 'TestResolveBrokerSelectionFailsClosedWhenExplicitUnixSocketIsNotSocket|TestResolveBrokerSelectionFailsClosedWhenCompatUnixSocketMissing|TestResolveBrokerSelectionFailsClosedWhenCompatUnixSocketIsNotSocket|TestResolveBrokerSelectionFailsClosedWhenRoleSocketMissingAndNoExplicitBroker|TestResolveBrokerSelectionExplicitBrokerURLWinsOverLocalSocketDefaults'
	go test ./cmd/promptlockd -run 'TestRegisterAgentRoutesToExposesOnlyAgentEndpoints|TestRegisterOperatorRoutesToExposesOnlyOperatorEndpoints|TestApproveRejectsMalformedJSON|TestDenyRejectsMalformedJSON|TestCancelRejectsMalformedJSON|TestConfigureControlPlaneUseCasesInjectsAmbientEnvFromBoundary|TestConfigureControlPlaneUseCasesDoesNotFallBackToProcessEnvWhenBoundaryInjectionMissing'
	go test ./internal/app -run 'TestExecuteWithLeaseUseCaseDoesNotInheritAmbientEnvWithoutInjection|TestHostDockerExecuteUseCaseDoesNotInheritAmbientEnvWithoutInjection'

docs:
	@test -f AGENTS.md
	@test -f docs/README.md
	@test -f docs/CONTRACT.md
	@test -f docs/architecture/ARCHITECTURE.md
	@test -f docs/architecture/CONFORMANCE.md
	@test -f docs/standards/ENGINEERING-STANDARDS.md
	@test -f docs/operations/HARDENED-SMOKE.md
	@test -f docs/operations/REAL-E2E-HOST-CONTAINER.md
	@test -f docs/compatibility/MCP-CONFORMANCE-MATRIX.md
	@test -f docs/decisions/README.md
	@test -f docs/decisions/INDEX.md
	@test -f docs/plans/README.md
	@test -f docs/plans/ACTIVE-PLAN.md
	@test -f docs/plans/BACKLOG.md
	@test -f docs/plans/status/PRODUCTION-READINESS-STATUS.json
	@test -f docs/plans/checklists/BETA-READINESS.md
	@test -f CHANGELOG.md

validate-changelog:
	go run ./cmd/promptlock-validate-changelog

hygiene:
	bash scripts/validate_repo_hygiene.sh
	bash scripts/validate_hygiene_portability.sh

validate-final: lint toolchain-guard vet race security security-redteam arch-conformance docs validate-changelog hygiene production-readiness-gate test
	@echo "Final validation gate passed."

ci: validate-final

e2e-compose:
	chmod +x scripts/docker_compose.sh
	scripts/docker_compose.sh -f docker-compose.e2e.yml up --build --abort-on-container-exit --exit-code-from e2e-runner
	scripts/docker_compose.sh -f docker-compose.e2e.yml down -v

setup-local-docker:
	go run ./cmd/promptlock setup

build-agent-lab:
	docker build --target agent-lab -t promptlock-agent-lab .

demo-print-github-token:
	@printf '%s\n' "$$GITHUB_TOKEN"

demo-run-env-showcase-tests:
	@PROMPTLOCK_DEMO_REQUIRE=1 \
		PROMPTLOCK_DEMO_MODE=env-showcase \
		PROMPTLOCK_DEMO_ACTOR=make-target \
		go test ./demo-envs/showcase -run 'TestPromptLockEnvShowcaseToken|TestPromptLockEnvShowcaseMetadata' -count=1 -v

release-package:
	@test -n "$(VERSION)" || (echo "Usage: make release-package VERSION=vX.Y.Z" && exit 1)
	chmod +x scripts/release-package.sh
	scripts/release-package.sh "$(VERSION)"
