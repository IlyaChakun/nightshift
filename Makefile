# Tools come from mise.toml (`mise install --locked`).
GO ?= go

.PHONY: verify fmt gen-check generate lint test vuln secrets docs ci-lint repo-check

## verify: everything that must be green before a PR leaves draft
verify: fmt gen-check lint test vuln secrets docs ci-lint repo-check

fmt:
	golangci-lint fmt --diff

## generate: regenerate typed book queries after changing internal/book/migrations or queries
generate:
	sqlc generate

gen-check:
	sqlc diff

lint:
	golangci-lint run ./...

test:
	$(GO) test -race -count=1 ./...

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
