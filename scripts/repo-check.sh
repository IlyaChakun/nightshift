#!/usr/bin/env bash
# Repository rules that no off-the-shelf linter checks.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0

# Handoff documents stay short so they do not flood an agent's context.
check_lines() {
  local file=$1 max=$2 n
  [[ -f $file ]] || return 0
  n=$(wc -l <"$file")
  if ((n > max)); then
    echo "repo-check: $file has $n lines, limit is $max" >&2
    fail=1
  fi
}
check_lines AGENTS.md 120
check_lines docs/plan.md 150
check_lines docs/status.md 40

# The core knows no tenants and no concrete services (AGENTS.md, rule 1).
names='\b(linear|jira|confluence|argo ?cd|sonar(qube)?|codeql|splunk|slack)\b'
if [[ -d internal ]]; then
  if grep -rniE "$names" internal --include='*.go' \
    --exclude-dir=adapters --exclude-dir=testdata; then
    echo "repo-check: service names found in the core; move them to internal/adapters" >&2
    fail=1
  fi
fi

# docs/design.html is a view of docs/design.md: every term it names must exist in the design.
if [[ -f docs/design.html ]]; then
  terms=$( {
    grep -oE '(data-term="|term: "|st: ")[^"]+' docs/design.html | sed -E 's/^(data-term="|term: "|st: ")//'
    sed -n '/^const STATES = \[/,/\];/p' docs/design.html | grep -oE '"[a-z_]+"' | tr -d '"'
  } | sort -u)
  while IFS= read -r term; do
    if ! grep -qF -- "$term" docs/design.md; then
      echo "repo-check: docs/design.html names \"$term\", which is not in docs/design.md" >&2
      fail=1
    fi
  done <<<"$terms"
fi

# Every compose secret is documented for the person who sets up an instance.
if [[ -f deploy/docker-compose.yml ]]; then
  for secret in $(sed -n '/^secrets:/,/^[^ ]/p' deploy/docker-compose.yml | grep -oE '^  [a-z0-9_]+:' | tr -d ' :'); do
    if ! grep -qF -- "\`$secret\`" docs/secrets.md 2>/dev/null; then
      echo "repo-check: compose secret \"$secret\" is not documented in docs/secrets.md" >&2
      fail=1
    fi
  done
fi

exit "$fail"
