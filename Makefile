# Tools come from mise.toml (`mise install --locked`).
GO ?= go

.PHONY: verify fmt lint test vuln secrets docs ci-lint repo-check

## verify: everything that must be green before a PR leaves draft
verify: fmt lint test vuln secrets docs ci-lint repo-check

fmt:
	golangci-lint fmt --diff

lint:
	golangci-lint run ./...

test:
	$(GO) test -race -count=1 ./...

vuln:
	govulncheck ./...
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
