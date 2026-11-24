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
check_lines LEARNINGS.md 100
# Package notes for agents: what the package owns and its traps, nothing more.
for f in internal/*/AGENTS.md; do check_lines "$f" 40; done

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

# Covered design sections must not change without a look at the page.
./scripts/design-sync.sh || fail=1

exit "$fail"
