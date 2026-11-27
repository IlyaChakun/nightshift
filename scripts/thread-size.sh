#!/usr/bin/env bash
# Says whether this agent thread has reached a handoff trigger of docs/plan.md ("How threads
# are run"): context near 250k tokens, about 10 merged PRs, or about 4 hours. It reads the
# Claude Code transcript of this session and asks GitHub for the PRs this session merged.
# Exit 1 means hand off now (skill finish-task); exit 0 also covers "cannot tell", which it
# says, so a missing tool never sends a thread away.
#
#   scripts/thread-size.sh [--transcript <file.jsonl>] [--merged <n>]
set -euo pipefail

usage() { echo "usage: scripts/thread-size.sh [--transcript <file.jsonl>] [--merged <n>]" >&2; exit 2; }
transcript= merged=
while (($#)); do
  case $1 in
    --transcript) (($# >= 2)) || usage; transcript=$2; shift 2 ;;
    --merged) (($# >= 2)) && [[ $2 =~ ^([0-9]+|unknown)$ ]] || usage; merged=$2; shift 2 ;;
    *) usage ;;
  esac
done
max_context=250000 max_merged=10 max_hours=4
unknown() { echo "thread-size: $1; check the triggers by hand" >&2; exit 0; }

# This session's transcript; the newest one only when the session id is not set.
if [[ -z $transcript ]]; then
  if [[ -n ${CLAUDE_CODE_SESSION_ID:-} ]]; then
    transcript=$(ls "$HOME"/.claude/projects/*/"$CLAUDE_CODE_SESSION_ID".jsonl 2>/dev/null | head -n 1 || true)
  else
    # shellcheck disable=SC2012 # newest file by mtime; ls -t works on GNU and macOS alike
    transcript=$(ls -t "$HOME"/.claude/projects/*/*.jsonl 2>/dev/null | head -n 1 || true)
  fi
fi
[[ -n $transcript && -f $transcript ]] || unknown "no transcript found"
command -v jq >/dev/null || unknown "jq is not installed"

# The context of the last model call is what every next call re-reads. Broken lines (a line
# still being written) are skipped; jq does the date arithmetic, so GNU and BSD date agree.
summary=$(jq -cR 'fromjson? // empty' "$transcript" 2>/dev/null | jq -rs '
  [.[] | select(.type == "assistant" and .message.usage != null)] as $a
  | ([.[] | .timestamp // empty | strings] | first) as $t
  | if ($a | length) == 0 or $t == null then empty else
    ($a | last | .message.usage) as $u
    | "\($t) \((now - ($t | sub("\\.[0-9]+"; "") | fromdate)) / 3600 | floor) \($a | map(.message.id) | unique | length) \(($u.input_tokens // 0) + ($u.cache_read_input_tokens // 0) + ($u.cache_creation_input_tokens // 0))"
    end' 2>/dev/null || true)
[[ -n $summary ]] || unknown "no model calls with usage and timestamps in $transcript"
read -r start hours calls context <<<"$summary"

# PRs this session merged: those from its branch, and those whose body carries the session
# link (AGENTS.md attribution). Threads drop the link often (13 of 30 PRs on 2026-11-27),
# so the branch counts too; a PR found both ways counts once.
if [[ -z $merged ]]; then
  merged=unknown
  session=${CLAUDE_CODE_REMOTE_SESSION_ID:-}
  branch=$(git branch --show-current 2>/dev/null || true)
  repo=$(git remote get-url origin 2>/dev/null | sed -E 's#.*github.com[:/]##; s#\.git$##; s#^git/##' || true)
  if [[ -n $repo ]] && [[ -n ${session#cse_} || ( -n $branch && $branch != main ) ]] && command -v gh >/dev/null; then
    base="repo:$repo is:pr is:merged merged:>=${start%%T*}"
    prs() { gh api -X GET search/issues -f q="$base $1" -f per_page=100 --jq '.items[].number' 2>/dev/null; }
    # Either query failing means "cannot tell", never a smaller count.
    found= ok=true
    if [[ -n ${session#cse_} ]]; then found+=$(prs "\"session_${session#cse_}\" in:body")$'\n' || ok=false; fi
    if [[ -n $branch && $branch != main ]]; then found+=$(prs "head:$branch")$'\n' || ok=false; fi
    $ok && merged=$(sort -u <<<"$found" | grep -c . || true)
  fi
fi

echo "thread-size: $calls model calls, context $context tokens, $merged PRs merged by this session, ${hours}h since $start"
over=
((context >= max_context)) && over="$over, context ≥ $max_context"
[[ $merged =~ ^[0-9]+$ ]] && ((merged >= max_merged)) && over="$over, ≥ $max_merged merged PRs"
((hours >= max_hours)) && over="$over, ≥ ${max_hours}h"
if [[ -n $over ]]; then
  echo "thread-size: hand off now (${over#, }): skill finish-task, \"Thread size and handoff\"" >&2
  exit 1
fi
