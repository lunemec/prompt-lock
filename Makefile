.PHONY: help lint test security docs validate-changelog validate-final ci

help:
	@echo "Targets: lint test security docs validate-changelog validate-final ci"

lint:
	bash -n scripts/secretctl.sh scripts/human-approve.sh
	python3 -m py_compile scripts/mock-broker.py

test:
	go test ./...

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
