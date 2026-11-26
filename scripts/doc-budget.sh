#!/usr/bin/env bash
# Word budgets per kind of document (skill write-docs). A file over its kind's max fails, unless
# scripts/doc-budget-baseline.txt lists it: then it may not grow past the listed count, and the
# line must go once the file fits. Usage: doc-budget.sh [repo root], default the repository.
set -euo pipefail
cd "${1:-$(dirname "$0")/..}"
baseline=scripts/doc-budget-baseline.txt
fail=0

# Prints the kind and max of a path, or nothing when no budget applies.
kind_of() {
  case $1 in
    docs/design.md) echo "design-doc 10000" ;;
    docs/adr/*.md) echo "adr 1000" ;;
    docs/retro/*.md) echo "retro 1000" ;;
    docs/research/*.md) echo "research-note 1500" ;;
    docs/runbook.md | docs/setup.md) echo "runbook 4000" ;;
    docs/secrets.md | docs/decisions.md | docs/prior-art.md) echo "reference 3000" ;;
    docs/live-checks/*.md) echo "plan-item 800" ;;
    .agents/skills/*/SKILL.md | skills/own/*/SKILL.md) echo "skill 1000" ;;
  esac
}

declare -A base=()
if [[ -f $baseline ]]; then
  while read -r path words _; do
    [[ -z $path || $path == \#* ]] && continue
    base[$path]=$words
    if [[ ! -f $path ]]; then
      echo "doc-budget: $baseline lists $path, which does not exist; delete the line" >&2
      fail=1
    fi
  done <"$baseline"
fi

shopt -s nullglob
for f in docs/*.md docs/adr/*.md docs/retro/*.md docs/research/*.md docs/live-checks/*.md \
  .agents/skills/*/SKILL.md skills/own/*/SKILL.md; do
  read -r kind max <<<"$(kind_of "$f")" || true
  [[ -n ${max:-} ]] || continue
  n=$(LC_ALL=C.UTF-8 wc -w <"$f") # the CI locale, as in repo-check.sh
  limit=${base[$f]:-}
  if [[ -n $limit ]]; then
    if ((n <= max)); then
      echo "doc-budget: $f has $n words and fits its $kind max of $max; delete its line in $baseline" >&2
      fail=1
    elif ((n > limit)); then
      echo "doc-budget: $f has $n words; it is over its $kind max of $max and may not grow past $limit (skill write-docs: shrink pass, or split by topic)" >&2
      fail=1
    fi
  elif ((n > max)); then
    echo "doc-budget: $f has $n words, the max for kind $kind is $max (skill write-docs: shrink pass, or split by topic)" >&2
    fail=1
  fi
  max=
done
exit "$fail"
