.PHONY: help lint test fuzz security security-redteam security-redteam-live security-redteam-live-hardened hardened-smoke mcp-conformance-report production-readiness-gate leak-guard ci-redteam-full arch-conformance docs validate-changelog hygiene validate-final ci e2e-compose release-package

help:
	@echo "Targets: lint test fuzz security security-redteam security-redteam-live security-redteam-live-hardened hardened-smoke mcp-conformance-report production-readiness-gate leak-guard ci-redteam-full arch-conformance docs validate-changelog validate-final ci e2e-compose release-package"

lint:
	bash -n scripts/secretctl.sh scripts/human-approve.sh
	python3 -m py_compile scripts/mock-broker.py

test:
	go test ./...

fuzz:
	go test ./cmd/promptlock-mcp -run=^$$ -fuzz=FuzzParseAndValidateExecArgs -fuzztime=5s
	go test ./cmd/promptlockd -run=^$$ -fuzz=FuzzValidateExecuteCommand -fuzztime=5s

security:
	python3 scripts/validate_security_basics.py

security-redteam:
	bash scripts/run_redteam_e2e.sh

security-redteam-live:
	mkdir -p reports
	python3 scripts/run_redteam_live.py reports/redteam-live.json dev

security-redteam-live-hardened:
	mkdir -p reports
	python3 scripts/run_redteam_live.py reports/redteam-live-hardened.json hardened

hardened-smoke:
	bash scripts/run_hardened_smoke.sh

mcp-conformance-report:
	bash scripts/mcp_conformance_report.sh

production-readiness-gate:
	go run ./cmd/promptlock-readiness-check --file docs/plans/PRODUCTION-READINESS-STATUS.json --require-p0

leak-guard:
	bash scripts/validate_no_secret_leaks.sh

ci-redteam-full: validate-final security-redteam-live security-redteam-live-hardened mcp-conformance-report leak-guard
	@echo "Full red-team CI profile passed."

arch-conformance:
	bash scripts/verify_architecture_conformance.sh

docs:
	@test -f AGENTS.md
	@test -f docs/CONTRACT.md
	@test -f docs/architecture/ARCHITECTURE.md
	@test -f docs/architecture/CONFORMANCE.md
	@test -f docs/standards/ENGINEERING-STANDARDS.md
	@test -f docs/operations/MTLS-HARDENED.md
	@test -f docs/operations/HARDENED-SMOKE.md
	@test -f docs/compatibility/MCP-CONFORMANCE-MATRIX.md
	@test -f docs/decisions/README.md
	@test -f CHANGELOG.md

validate-changelog:
	python3 scripts/validate_changelog.py

hygiene:
	bash scripts/validate_repo_hygiene.sh

validate-final: lint security security-redteam arch-conformance docs validate-changelog hygiene test
	@echo "Final validation gate passed."

ci: validate-final

e2e-compose:
	docker compose -f docker-compose.e2e.yml up --build --abort-on-container-exit --exit-code-from e2e-runner
	docker compose -f docker-compose.e2e.yml down -v

release-package:
	@test -n "$(VERSION)" || (echo "Usage: make release-package VERSION=vX.Y.Z" && exit 1)
	chmod +x scripts/release-package.sh
	scripts/release-package.sh "$(VERSION)"
