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
# A line limit alone is gamed by long lines, so the handoff files also have a word limit.
check_words() {
  local file=$1 max=$2 n
  [[ -f $file ]] || return 0
  n=$(LC_ALL=C.UTF-8 wc -w <"$file") # the CI locale: an em-dash counts as a word
  if ((n > max)); then
    echo "repo-check: $file has $n words, limit is $max (skill write-docs: cut, do not compress)" >&2
    fail=1
  fi
}
check_lines AGENTS.md 120
check_lines docs/plan.md 60
check_lines docs/status.md 40
check_lines LEARNINGS.md 100
check_words AGENTS.md 1800
check_words docs/status.md 600
check_words docs/plan.md 800
check_words LEARNINGS.md 700
# One folder per phase or stream under docs/plan/, owned by the session that works on it. Its
# README.md holds the status and the items and has its own limit, so one phase cannot starve
# another, and a link from docs/plan.md, so none is forgotten.
for f in docs/plan/*.md docs/plan/*/README.md; do
  [[ -f $f ]] || continue
  check_lines "$f" 200
  check_words "$f" 4000
  if ! grep -qF -- "(${f#docs/})" docs/plan.md; then
    echo "repo-check: docs/plan.md does not link $f" >&2
    fail=1
  fi
done
while IFS= read -r f; do
  if [[ ! -f docs/$f ]]; then
    echo "repo-check: docs/plan.md links docs/$f, which does not exist" >&2
    fail=1
  fi
done < <(grep -oE '\(plan/[A-Za-z0-9_./-]+\.md\)' docs/plan.md | tr -d '()')
# Any other file of a plan folder is one item written by a session that does not own the README.md:
# a new file never conflicts. It is short and says where it stands.
for d in docs/plan/*/; do
  [[ -d $d ]] || continue
  if [[ ! -f ${d}README.md ]]; then
    echo "repo-check: $d has no README.md" >&2
    fail=1
  fi
  for f in "$d"*.md; do
    [[ -f $f && $f != "${d}README.md" ]] || continue
    check_lines "$f" 80
    check_words "$f" 800
    if ! grep -qE '^State: (proposed|open|done)$' "$f"; then
      echo "repo-check: $f needs a line 'State: proposed', 'State: open' or 'State: done'" >&2
      fail=1
    fi
  done
done
# Individual live checks have one status after the pending or completed check.
for f in docs/live-checks/*.md; do
  [[ -f $f ]] || continue
  if [[ $(grep -c '^Status:' "$f") != 1 ]] || ! grep -Eq '^Status: (pending|(passed|failed) .+)$' "$f"; then
    echo "repo-check: $f needs one Status line with pending, passed with evidence, or failed with evidence" >&2
    fail=1
  fi
done
# Two branches can each add the next ADR number under different names, and git merges that
# without a conflict: one number must name one ADR.
dup=$(ls docs/adr 2>/dev/null | sed -nE 's/^([0-9]{4})-.*/\1/p' | sort | uniq -d | tr '\n' ' ')
if [[ -n $dup ]]; then
  echo "repo-check: docs/adr has more than one ADR with the number: $dup(renumber the one merged last)" >&2
  fail=1
fi
# Package notes for agents: what the package owns and its traps, nothing more.
for f in internal/*/AGENTS.md; do check_lines "$f" 40; done

# A binary built into the tree must never be committed again.
while IFS= read -r f; do
  if [[ -f $f ]] && (($(wc -c <"$f") > 1048576)); then
    echo "repo-check: $f is larger than 1 MiB; build outputs and data do not belong in git" >&2
    fail=1
  fi
done < <(git ls-files)

# Postgres for tests comes only from internal/testutil/pgtest, which shares one container per test
# binary. A container per test once made the test job take 120 s (see LEARNINGS.md, "Slow CI").
if grep -rnE 'postgres\.Run\(|testcontainers\.(Generic)?Container|GenericContainer\(' internal cmd --include='*.go' \
  --exclude-dir=pgtest; then
  echo "repo-check: start Postgres for a test with pgtest.Start, not a container of your own" >&2
  fail=1
fi

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

# The phases table of docs/design.html follows docs/plan.md and the phase headings of the design.
# The page carries only the current phase, so a tick in a plan file never needs an edit of the page.
if [[ -f docs/design.html && -f docs/plan.md ]]; then
  plan_phase=$(sed -nE 's/^## Current phase: ([0-9]+) .*/\1/p' docs/plan.md | head -1)
  page_plan=$(grep -E '^const PLAN = ' docs/design.html || true)
  if [[ $page_plan != "const PLAN = { phase: $plan_phase };" ]]; then
    echo "repo-check: docs/design.html PLAN differs from docs/plan.md" \
      "(want phase: $plan_phase); update it in the same PR" >&2
    fail=1
  fi
  while IFS=$'\t' read -r n title; do
    heading=$(grep -m1 "^### Phase $n\. " docs/design.md | sed -E 's/^### Phase [0-9]+\. //; s/`//g' || true)
    if [[ $heading != "$title" ]]; then
      echo "repo-check: docs/design.html names phase $n \"$title\"; design section 22 has \"$heading\"" >&2
      fail=1
    fi
  done < <(sed -nE 's/^  \[([0-9]+), "([^"]+)", ".*/\1\t\2/p' docs/design.html)
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

# Dev skills live in .agents/skills (vendor-neutral); .claude/skills points there for Claude Code.
# Each skill is a folder whose SKILL.md names itself after the folder and says when to use it.
if [[ -d .agents/skills ]]; then
  [[ $(readlink .claude/skills 2>/dev/null) == ../.agents/skills ]] || {
    echo "repo-check: .claude/skills must be a symlink to ../.agents/skills" >&2
    fail=1
  }
  for d in .agents/skills/*/; do
    name=$(basename "$d")
    if [[ ! -f $d/SKILL.md ]] || ! grep -qx "name: $name" "$d/SKILL.md" ||
      ! grep -q '^description: .\{20,\}' "$d/SKILL.md"; then
      echo "repo-check: $d/SKILL.md needs front matter with name: $name and a description" >&2
      fail=1
    fi
  done
fi

# A merge conflict committed by mistake breaks whatever reads the file; no tracked file starts
# a line with a conflict marker.
if markers=$(git grep -nE '^(<<<<<<<|>>>>>>>) ' -- . 2>/dev/null); then
  echo "repo-check: conflict markers in tracked files:" >&2
  echo "$markers" >&2
  fail=1
fi

# The idle pull request sweep keeps its behaviour (stub `gh`, no network).
./scripts/test-pr-sweep.sh || fail=1
./scripts/test-pr-gate.sh || fail=1

# Word budgets per kind of document (skill write-docs), and their own scenarios.
./scripts/test-doc-budget.sh || fail=1
./scripts/doc-budget.sh || fail=1

# Covered design sections must not change without a look at the page.
./scripts/design-sync.sh || fail=1

exit "$fail"
