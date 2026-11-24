# Tools come from mise.toml (`mise install --locked`).
GO ?= go

.PHONY: verify tools sql-lint docker-lint fmt gen-check generate lint test vuln secrets docs ci-lint repo-check

## verify: everything that must be green before a PR leaves draft
verify: tools fmt gen-check sql-lint docker-lint lint test vuln secrets docs ci-lint repo-check

## tools: fail fast when the local linter differs from the pin in mise.toml (CI would fail on it later)
tools:
	@want=$$(sed -n 's/^golangci-lint = "\(.*\)"/\1/p' mise.toml); \
	have=$$(golangci-lint version 2>&1 | sed -n 's/.*version \([0-9.]*\).*/\1/p'); \
	[ "$$want" = "$$have" ] || { echo "make tools: golangci-lint is $$have, mise.toml pins $$want; run 'mise install'"; exit 1; }

fmt:
	golangci-lint fmt --diff

## generate: regenerate typed book queries after changing internal/book/migrations or queries
generate:
	sqlc generate

gen-check:
	sqlc diff

## sql-lint: unsafe migration patterns (rules in .squawk.toml)
sql-lint:
	squawk internal/book/migrations/*.sql

## docker-lint: Dockerfile practices (rules in .hadolint.yaml)
docker-lint:
	hadolint Dockerfile

lint:
	golangci-lint run ./...

# A random shuffle order finds tests that depend on each other; the seed is printed to reproduce.
test:
	seed=$$(date +%s); echo "test shuffle seed: $$seed"; $(GO) test -race -count=1 -shuffle=$$seed ./...

# Fail on any known advisory in a required module, even if the code is never called.
# govulncheck exits non-zero only for called code, so its JSON findings are checked too.
vuln:
	govulncheck -show verbose ./...
	if govulncheck -format json ./... | grep -q '"finding"'; then \
		echo "make vuln: known advisories in required modules, see the report above"; exit 1; \
	fi
	osv-scanner scan source --recursive .

secrets:
	gitleaks git --redact --no-banner .
	gitleaks dir --redact --no-banner .

docs:
	markdownlint-cli2

ci-lint:
	actionlint
	zizmor --offline .github

repo-check:
	./scripts/repo-check.sh
