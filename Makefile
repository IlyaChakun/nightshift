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

# Fail on any known advisory in a required module, even if the code is never called.
# -scan module takes no patterns, and the module root has no Go files, hence -C.
vuln:
	govulncheck -C cmd/nightshift -scan module
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
