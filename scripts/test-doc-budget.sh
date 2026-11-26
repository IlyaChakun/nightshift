#!/usr/bin/env bash
# Offline scenarios for scripts/doc-budget.sh on a scratch tree.
set -euo pipefail
cd "$(dirname "$0")/.."
script=$PWD/scripts/doc-budget.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/docs/adr" "$tmp/scripts"
words() { printf "word %.0s" $(seq "$1") >"$2"; }
expect() { # expect <0|1> <name> [text the output must contain]
  local want=$1 name=$2 text=${3:-} got=0 out
  out=$("$script" "$tmp" 2>&1) || got=1
  if [[ $got != "$want" || ( -n $text && $out != *"$text"* ) ]]; then
    echo "test-doc-budget: $name: exit $got, want $want; output: $out" >&2
    exit 1
  fi
}
words 900 "$tmp/docs/adr/0001-a.md"
expect 0 "an ADR under its max passes"
words 1100 "$tmp/docs/adr/0001-a.md"
expect 1 "an ADR over its max fails" "max for kind adr is 1000"
echo "docs/adr/0001-a.md 1100" >"$tmp/scripts/doc-budget-baseline.txt"
expect 0 "a baselined file at its count passes"
words 1101 "$tmp/docs/adr/0001-a.md"
expect 1 "a baselined file may not grow" "may not grow past 1100"
words 1000 "$tmp/docs/adr/0001-a.md"
expect 1 "a baseline line for a file that fits must go" "delete its line"
rm "$tmp/docs/adr/0001-a.md"
expect 1 "a baseline line for a missing file must go" "does not exist"
