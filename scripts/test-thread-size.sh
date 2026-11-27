#!/usr/bin/env bash
# Offline scenarios for scripts/thread-size.sh on a scratch transcript.
set -euo pipefail
cd "$(dirname "$0")/.."
script=$PWD/scripts/thread-size.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
call() { # call <id> <cache read tokens> <timestamp>
  printf '{"type":"assistant","timestamp":"%s","message":{"id":"%s","usage":{"input_tokens":1,"cache_read_input_tokens":%s,"cache_creation_input_tokens":9}}}\n' "$3" "$1" "$2"
}
now=$(date -u +%Y-%m-%dT%H:%M:%S.000Z)
expect() { # expect <0|1> <name> <merged> [text the output must contain]
  local want=$1 name=$2 got=0 out
  out=$("$script" --transcript "$tmp/t.jsonl" --merged "$3" 2>&1) || got=1
  if [[ $got != "$want" || ( -n ${4:-} && $out != *"$4"* ) ]]; then
    echo "test-thread-size: $name: exit $got, want $want; output: $out" >&2
    exit 1
  fi
}
{ call a 90000 "$now"; call a 90000 "$now"; call b 120000 "$now"; } >"$tmp/t.jsonl"
expect 0 "a small young thread goes on" 2 "2 model calls, context 120010 tokens"
expect 1 "ten merged PRs hand off" 10 "≥ 10 merged PRs"
expect 0 "an unknown merge count does not fail" unknown "unknown PRs merged"
call c 260000 "$now" >>"$tmp/t.jsonl"
expect 1 "the last call's context counts" 0 "context ≥ 250000"
{ call a 1000 "2020-01-01T00:00:00.000Z"; call b 1000 "$now"; } >"$tmp/t.jsonl"
expect 1 "an old thread hands off" 0 "≥ 4h"
echo "test-thread-size: ok"
