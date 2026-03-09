.PHONY: help lint test fuzz security security-redteam security-redteam-live security-redteam-live-hardened hardened-smoke real-e2e-smoke mcp-conformance-report production-readiness-gate leak-guard storage-fsync-preflight storage-fsync-report storage-fsync-validate storage-fsync-release-gate ci-redteam-full arch-conformance docs validate-changelog hygiene validate-final ci e2e-compose release-package

FSYNC_REPORT ?= reports/storage-fsync-report.json

help:
	@echo "Targets: lint test fuzz security security-redteam security-redteam-live security-redteam-live-hardened hardened-smoke real-e2e-smoke mcp-conformance-report production-readiness-gate leak-guard storage-fsync-preflight storage-fsync-report storage-fsync-validate storage-fsync-release-gate ci-redteam-full arch-conformance docs validate-changelog validate-final ci e2e-compose release-package"

lint:
	bash -n scripts/secretctl.sh scripts/human-approve.sh

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
	go run ./cmd/promptlock-readiness-check --file docs/plans/status/PRODUCTION-READINESS-STATUS.json --require-p0

leak-guard:
	bash scripts/validate_no_secret_leaks.sh

storage-fsync-preflight:
	@test -n "$(MOUNT_DIR)" || (echo "Usage: make storage-fsync-preflight MOUNT_DIR=/path/to/mount" && exit 1)
	go run ./cmd/promptlock-storage-fsync-check --dir "$(MOUNT_DIR)"

storage-fsync-report:
	@test -n "$(MOUNT_DIRS)" || (echo "Usage: make storage-fsync-report MOUNT_DIRS=/path/a,/path/b [FSYNC_REPORT=reports/storage-fsync-report.json]" && exit 1)
	@mkdir -p "$(dir $(FSYNC_REPORT))"
	go run ./cmd/promptlock-storage-fsync-check --dir-list "$(MOUNT_DIRS)" --json > "$(FSYNC_REPORT)"
	@echo "storage fsync report written to $(FSYNC_REPORT)"

storage-fsync-validate:
	@test -n "$(FSYNC_REPORT)" || (echo "Usage: make storage-fsync-validate FSYNC_REPORT=reports/storage-fsync-report.json" && exit 1)
	@test -f "$(FSYNC_REPORT)" || (echo "Missing fsync report file: $(FSYNC_REPORT)" && exit 1)
	go run ./cmd/promptlock-storage-fsync-validate --file "$(FSYNC_REPORT)"

storage-fsync-release-gate:
	@test -n "$(MOUNT_DIRS)" || (echo "Usage: make storage-fsync-release-gate MOUNT_DIRS=/path/a,/path/b [FSYNC_REPORT=reports/storage-fsync-report.json]" && exit 1)
	@mkdir -p "$(dir $(FSYNC_REPORT))"
	go run ./cmd/promptlock-storage-fsync-check --dir-list "$(MOUNT_DIRS)" --json > "$(FSYNC_REPORT)"
	go run ./cmd/promptlock-storage-fsync-validate --file "$(FSYNC_REPORT)"
	@echo "storage fsync release gate passed with report $(FSYNC_REPORT)"

ci-redteam-full: validate-final security-redteam-live security-redteam-live-hardened mcp-conformance-report real-e2e-smoke leak-guard
	@echo "Full red-team CI profile passed."

arch-conformance:
	bash scripts/verify_architecture_conformance.sh

docs:
	@test -f AGENTS.md
	@test -f docs/README.md
	@test -f docs/CONTRACT.md
	@test -f docs/architecture/ARCHITECTURE.md
	@test -f docs/architecture/CONFORMANCE.md
	@test -f docs/standards/ENGINEERING-STANDARDS.md
	@test -f docs/operations/MTLS-HARDENED.md
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

validate-final: lint security security-redteam arch-conformance docs validate-changelog hygiene production-readiness-gate test
	@echo "Final validation gate passed."

ci: validate-final

e2e-compose:
	docker compose -f docker-compose.e2e.yml up --build --abort-on-container-exit --exit-code-from e2e-runner
	docker compose -f docker-compose.e2e.yml down -v

release-package:
	@test -n "$(VERSION)" || (echo "Usage: make release-package VERSION=vX.Y.Z" && exit 1)
	chmod +x scripts/release-package.sh
	scripts/release-package.sh "$(VERSION)"
