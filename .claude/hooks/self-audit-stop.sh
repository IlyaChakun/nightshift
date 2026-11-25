#!/usr/bin/env bash
# Stop hook: a session that changed code may not stop without a self-audit block, and not while the
# promise tests (cmd/nightshift/promises_test.go, design_test.go) are red because of its change.
# Exit 2 blocks the stop and feeds stderr to the agent; exit 0 lets it stop.
#
# It never traps a session:
#   - it blocks once per stop (the harness sets stop_hook_active on the continuation);
#   - it passes when the change touches no code, prompts, schemas or design (documents only);
#   - it passes when every red test is red on origin/main too (not this change's fault);
#   - it passes when git, go, python3 or origin/main are missing.
# It checks that the block is present and filled, not that it is true. Rule: AGENTS.md, skill `self-audit`.
set -uo pipefail
cd "${CLAUDE_PROJECT_DIR:-$(dirname "$0")/../..}" || exit 0

payload=$(cat)
command -v python3 >/dev/null && command -v git >/dev/null || exit 0

field() { printf '%s' "$payload" | python3 -I -c 'import json,sys; v=json.load(sys.stdin).get(sys.argv[1],""); print(v if isinstance(v,str) else "1" if v else "")' "$1" 2>/dev/null; }
[[ -n $(field stop_hook_active) ]] && exit 0

base=$(git merge-base HEAD origin/main 2>/dev/null) || exit 0
changed=$({ git diff --name-only "$base"; git ls-files -o --exclude-standard; } 2>/dev/null | sort -u)
grep -qE '\.go$|^(internal|cmd|roles|skills|\.agents|schemas|standards|stacks|templates|deploy|\.github)/|^(AGENTS\.md|Makefile|docs/design\.md)$' <<<"$changed" || exit 0

problems=()

# 1. Promise tests: only failures this change added count.
test_cmd=${NIGHTSHIFT_STOP_HOOK_TEST_CMD:-"go test ./cmd/nightshift -run 'TestPrompts|TestRole|TestSchemaFields|TestDesign'"}
failing() { # prints the sorted failing test names in directory $1, or "build-or-other" when none is named
  local out names
  out=$(cd "$1" && eval "$test_cmd" 2>&1) && return 0
  names=$(grep -oE '^ *--- FAIL: [^ ]+' <<<"$out" | awk '{print $3}' | sort -u)
  printf '%s\n' "${names:-build-or-other}"
  return 1
}
if [[ -z ${NIGHTSHIFT_STOP_HOOK_TEST_CMD:-} ]]; then command -v go >/dev/null || test_cmd=true; fi
now=$(failing .)
if [[ $? -ne 0 ]]; then
  tmp=$(mktemp -d) && git worktree add -q --detach "$tmp/base" origin/main 2>/dev/null
  was=$(failing "$tmp/base")
  git worktree remove --force "$tmp/base" 2>/dev/null; rm -rf "$tmp"
  new=$(comm -23 <(printf '%s\n' "$now") <(printf '%s\n' "$was"))
  [[ -n $new ]] && problems+=("These promise tests are red because of your change (they pass on origin/main): $(tr '\n' ' ' <<<"$new")
   Run: $test_cmd
   Fix the code, prompt or schema they name. A real gap that you cannot close now goes into the allowlist in cmd/nightshift/promises_test.go with an open plan item.")
fi

# 2. A filled self-audit block in this session's own words.
if ! printf '%s' "$payload" | python3 -I -c '
import json, re, sys
p = json.load(sys.stdin)
def strings(x):
    if isinstance(x, str): yield x
    elif isinstance(x, list):
        for i in x: yield from strings(i)
    elif isinstance(x, dict):
        for i in x.values(): yield from strings(i)
texts = [p.get("last_assistant_message") or ""]
try:
    for line in open(p.get("transcript_path") or "/dev/null"):
        try: e = json.loads(line)
        except ValueError: continue
        if e.get("type") == "assistant": texts += list(strings(e.get("message", {}).get("content", [])))
except OSError: pass
ok = any(re.search(r"Self-audit", t) and re.search(r"^[ \t>*-]*Not verified:[ \t]*[^<\s]", t, re.M) for t in texts)
sys.exit(0 if ok else 1)' 2>/dev/null; then
  problems+=("No filled self-audit block in this session. Run the skill self-audit (.agents/skills/self-audit/SKILL.md), then put its block ('Self-audit' with the lines Claims and evidence, Promises checked, Not verified, Neighbours checked, Skeptical questions) in your report. 'Not verified:' must say what was not run live, or list the live checks.")
fi

((${#problems[@]} == 0)) && exit 0
{
  echo "Stop blocked by .claude/hooks/self-audit-stop.sh (AGENTS.md: Done means checked). Fix, then finish:"
  printf ' - %s\n' "${problems[@]}"
  echo "This blocks once per stop. If a point is wrong for this change, say so in your report instead of looping."
} >&2
exit 2
