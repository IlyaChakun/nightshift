#!/usr/bin/env bash
# Tests .claude/hooks/self-audit-stop.sh on a throwaway repository with a stubbed test command.
set -euo pipefail
hook=$(cd "$(dirname "$0")/.." && pwd)/.claude/hooks/self-audit-stop.sh
root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT
fail=0

repo=$root/repo
git init -q -b main "$repo"
git -C "$repo" -c user.email=t@t -c user.name=t commit -q --allow-empty -m init
git -C "$repo" update-ref refs/remotes/origin/main HEAD
git -C "$repo" checkout -q -b work

good='Self-audit
- Claims and evidence: x: ran y
- Not verified: the live stack'
placeholder='Self-audit
- Not verified: <what was not run live>'

# run <name> <want-exit> <payload-json> [KEY=VAL...]
run() {
  local name=$1 want=$2 payload=$3 got; shift 3
  got=0
  (cd "$repo" && printf '%s' "$payload" | env CLAUDE_PROJECT_DIR="$repo" "$@" "$hook" >"$root/out" 2>&1) || got=$?
  if [[ $got != "$want" ]]; then
    echo "FAIL $name: exit $got, want $want"; cat "$root/out"; fail=1
  fi
}
msg() { python3 -I -c 'import json,sys; print(json.dumps({"last_assistant_message": sys.argv[1]}))' "$1"; }
green=NIGHTSHIFT_STOP_HOOK_TEST_CMD=true
# red only in the working tree; the baseline worktree of origin/main is a directory named base
red="NIGHTSHIFT_STOP_HOOK_TEST_CMD=[[ \$PWD == */base ]] || { echo '--- FAIL: TestRoleKeysAreRead' >&2; false; }"

run "no change" 0 "$(msg '')" "$green"
echo '# notes' >"$repo/NOTES.md"
run "documents only" 0 "$(msg '')" "$green"
mkdir -p "$repo/internal/x" && echo 'package x' >"$repo/internal/x/x.go"
run "code, no block" 2 "$(msg 'done')" "$green"
grep -q 'self-audit' "$root/out" || { echo "FAIL: message does not name the skill"; fail=1; }
run "code, placeholder block" 2 "$(msg "$placeholder")" "$green"
run "code, filled block, green" 0 "$(msg "$good")" "$green"
run "stop_hook_active passes" 0 '{"stop_hook_active": true}' "$green"
run "new red test blocks" 2 "$(msg "$good")" "$red"
grep -q 'TestRoleKeysAreRead' "$root/out" || { echo "FAIL: message does not name the test"; fail=1; }
# the same test is red on origin/main: not this change's fault
run "red on main too" 0 "$(msg "$good")" "NIGHTSHIFT_STOP_HOOK_TEST_CMD=echo '--- FAIL: TestOld' >&2; false"
# a block in a tool call of the transcript counts (threads report through a tool)
printf '%s\n' "$(python3 -I -c 'import json,sys; print(json.dumps({"type":"assistant","message":{"content":[{"type":"tool_use","input":{"text":sys.argv[1]}}]}}))' "$good")" >"$root/t.jsonl"
run "block in transcript" 0 "{\"transcript_path\": \"$root/t.jsonl\"}" "$green"
# the skill text itself (a user or tool entry) is not a block
printf '%s\n' "$(python3 -I -c 'import json,sys; print(json.dumps({"type":"user","message":{"content":sys.argv[1]}}))' "$good")" >"$root/u.jsonl"
run "block only in a user entry" 2 "{\"transcript_path\": \"$root/u.jsonl\"}" "$green"
# regressions from the review: a bad payload never blocks, an empty label is not a block,
# and an unbuildable baseline does not put the blame on the change
run "empty payload" 0 '' "$green"
run "garbage payload" 0 'garbage' "$green"
run "empty bold label" 2 "$(msg $'Self-audit\n**Not verified:** \n')" "$green"
git -C "$repo" update-ref -d refs/remotes/origin/main
git -C "$repo" update-ref refs/remotes/origin/main "$(git -C "$repo" rev-parse HEAD)" # base = HEAD: worktree builds
mkdir -p "$root/bin"
printf '#!/usr/bin/env bash\n[[ $1 == worktree && $2 == add ]] && exit 128\nexec %s "$@"\n' "$(command -v git)" >"$root/bin/git"
chmod +x "$root/bin/git"
run "baseline cannot be built" 0 "$(msg "$good")" "PATH=$root/bin:$PATH" "$red"
exit $fail
