#!/usr/bin/env bash
# Offline scenarios for scripts/thread-size.sh on a scratch transcript, with a stub gh.
set -euo pipefail
cd "$(dirname "$0")/.."
script=$PWD/scripts/thread-size.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
call() { # call <id> <cache read tokens> <timestamp>
  printf '{"type":"assistant","timestamp":"%s","message":{"id":"%s","usage":{"input_tokens":1,"cache_read_input_tokens":%s,"cache_creation_input_tokens":9}}}\n' "$3" "$1" "$2"
}
now=$(date -u +%Y-%m-%dT%H:%M:%S.000Z)
expect() { # expect <exit> <name> <text the output must contain> [args...]
  local want=$1 name=$2 text=$3 got=0 out
  shift 3
  out=$("$script" "$@" 2>&1) || got=$?
  if [[ $got != "$want" || $out != *"$text"* ]]; then
    echo "test-thread-size: $name: exit $got, want $want; output: $out" >&2
    exit 1
  fi
}
t=(--transcript "$tmp/t.jsonl")
{ call a 90000 "$now"; call a 90000 "$now"; call b 120000 "$now"; } >"$tmp/t.jsonl"
expect 0 "a small young thread goes on" "2 model calls, context 120010 tokens" "${t[@]}" --merged 2
expect 1 "ten merged PRs hand off" "≥ 10 merged PRs" "${t[@]}" --merged 10
expect 0 "an unknown merge count does not fail" "unknown PRs merged" "${t[@]}" --merged unknown
printf '{"type":"assistant","timestamp":"%s","mess' "$now" >>"$tmp/t.jsonl"
expect 0 "a half-written last line is skipped" "context 120010 tokens" "${t[@]}" --merged 0
call c 260000 "$now" >"$tmp/t.jsonl"
expect 1 "the last call's context counts" "context ≥ 250000" "${t[@]}" --merged 0
{ call a 1000 "2020-01-01T00:00:00.000Z"; call b 1000 "$now"; } >"$tmp/t.jsonl"
expect 1 "an old thread hands off" "≥ 4h" "${t[@]}" --merged 0
echo '{"type":"summary"}' >"$tmp/t.jsonl"
expect 0 "a transcript without model calls cannot tell" "check the triggers by hand" "${t[@]}"
expect 0 "a missing transcript cannot tell" "no transcript found" --transcript "$tmp/none.jsonl"
expect 2 "a flag without a value is a usage error" "usage" --transcript
expect 2 "a malformed count is a usage error" "usage" "${t[@]}" --merged 1x

# The merge count comes from the PRs whose body carries this session's link.
mkdir -p "$tmp/bin" "$tmp/repo"
cat >"$tmp/bin/gh" <<'GH'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$GH_LOG"
case $* in *in:body*) [[ -z ${GH_FAIL_BODY:-} ]] || exit 1; printf '%s\n' 7 8 9 ;; *head:*) printf '%s\n' 9 10 ;; esac
GH
chmod +x "$tmp/bin/gh"
git -C "$tmp/repo" init -q -b claude/thread-x && git -C "$tmp/repo" remote add origin https://github.com/example/factory.git
call a 1000 "$now" >"$tmp/t.jsonl"
out=$(cd "$tmp/repo" && GH_LOG=$tmp/gh.log CLAUDE_CODE_REMOTE_SESSION_ID=cse_abc PATH="$tmp/bin:$PATH" "$script" "${t[@]}" 2>&1)
# Body link finds 7 8 9, the branch 9 10: four PRs, #9 counted once.
if [[ $out != *"4 PRs merged by this session"* ]] || ! grep -q '"session_abc" in:body' "$tmp/gh.log" ||
  ! grep -q 'head:claude/thread-x' "$tmp/gh.log"; then
  echo "test-thread-size: the session's merged PRs are not counted: $out; gh got: $(cat "$tmp/gh.log")" >&2
  exit 1
fi
out=$(cd "$tmp/repo" && GH_LOG=$tmp/gh.log GH_FAIL_BODY=1 CLAUDE_CODE_REMOTE_SESSION_ID=cse_abc PATH="$tmp/bin:$PATH" "$script" "${t[@]}" 2>&1)
[[ $out == *"unknown PRs merged"* ]] || { echo "test-thread-size: a failed query must give unknown, not a smaller count: $out" >&2; exit 1; }
rm -f "$tmp/gh.log"
out=$(cd "$tmp/repo" && GH_LOG=$tmp/gh.log PATH="$tmp/bin:$PATH" env -u CLAUDE_CODE_REMOTE_SESSION_ID "$script" "${t[@]}" 2>&1)
[[ $out == *"2 PRs merged by this session"* ]] || { echo "test-thread-size: a session without its link must count its branch: $out" >&2; exit 1; }
out=$(cd "$tmp" && env -u CLAUDE_CODE_REMOTE_SESSION_ID PATH="$tmp/bin:$PATH" "$script" "${t[@]}" 2>&1)
[[ $out == *"unknown PRs merged"* ]] || { echo "test-thread-size: no session or repo must give unknown: $out" >&2; exit 1; }
echo "test-thread-size: ok"
