.PHONY: help lint test fuzz security docs validate-changelog validate-final ci e2e-compose

help:
	@echo "Targets: lint test fuzz security docs validate-changelog validate-final ci e2e-compose"

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

docs:
	@test -f AGENTS.md
	@test -f docs/CONTRACT.md
	@test -f docs/architecture/ARCHITECTURE.md
	@test -f docs/standards/ENGINEERING-STANDARDS.md
	@test -f docs/decisions/README.md
	@test -f CHANGELOG.md

validate-changelog:
	python3 scripts/validate_changelog.py

validate-final: lint security docs validate-changelog test
	@echo "Final validation gate passed."

ci: validate-final

e2e-compose:
	docker compose -f docker-compose.e2e.yml up --build --abort-on-container-exit --exit-code-from e2e-runner
	docker compose -f docker-compose.e2e.yml down -v
