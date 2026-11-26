# Tools come from mise.toml (`mise install --locked`).
GO ?= go

# A shell that did not run `mise activate` (an agent's session, a script, a fresh container) finds an older
# system copy of a tool before the pinned one. Put the directories of the mise-installed tools first, so
# `make` uses the same versions as CI. Without mise, or when it cannot read mise.toml, PATH stays as it is.
MISE_BIN := $(shell command -v mise 2>/dev/null)
ifneq ($(MISE_BIN),)
MISE_PATHS := $(shell $(MISE_BIN) bin-paths 2>/dev/null | tr '\n' ':')
export PATH := $(MISE_PATHS)$(PATH)
endif

.PHONY: verify tools sql-lint docker-lint fmt gen-check generate lint test vuln secrets docs ci-lint repo-check drill

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
# mise.lock maps macOS arm64 to the x86_64 binary of the pinned release, which crashes there (docs/status.md).
# A local run whose binary does not start falls back to the official image of the same version (native
# linux/arm64). CI never falls back: a broken binary there is a red check.
docker-lint:
	@if hadolint --version >/dev/null 2>&1; then \
		hadolint Dockerfile; \
	elif [ -z "$$CI" ] && docker info >/dev/null 2>&1; then \
		ver=$$(sed -n 's/^hadolint = "\(.*\)"/\1/p' mise.toml); \
		[ -n "$$ver" ] || { echo "make docker-lint: cannot read the hadolint pin from mise.toml"; exit 1; }; \
		echo "make docker-lint: no working local hadolint, using hadolint/hadolint:v$$ver"; \
		docker run --rm -v "$$PWD:/work:ro" -w /work "hadolint/hadolint:v$$ver" /bin/hadolint Dockerfile; \
	else \
		echo "make docker-lint: hadolint is missing or does not start; run 'mise install', or start Docker for the local fallback"; exit 1; \
	fi

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

## drill: the live Done checks of phases 1 and 2 on a throwaway compose project (needs Docker; not part of verify)
drill:
	./scripts/drill.sh
