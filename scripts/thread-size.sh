#!/usr/bin/env bash
# Says whether this agent thread has reached a handoff trigger of docs/plan.md ("How threads
# are run"): context near 250k tokens, about 10 merged PRs from its branch, or about 4 hours.
# It reads the Claude Code transcript (newest under ~/.claude/projects unless given) and asks
# GitHub for the branch's merged PRs. Exit 1 means hand off now (skill finish-task).
#
#   scripts/thread-size.sh [--transcript <file.jsonl>] [--merged <n>]
set -euo pipefail

transcript= merged=
while (($#)); do
  case $1 in
    --transcript) transcript=$2; shift 2 ;;
    --merged) merged=$2; shift 2 ;;
    *) echo "usage: scripts/thread-size.sh [--transcript <file.jsonl>] [--merged <n>]" >&2; exit 2 ;;
  esac
done
max_context=250000 max_merged=10 max_hours=4

if [[ -z $transcript ]]; then
  # shellcheck disable=SC2012 # newest file by mtime; ls -t works on GNU and macOS alike
  transcript=$(ls -t "$HOME"/.claude/projects/*/*.jsonl 2>/dev/null | head -n 1 || true)
fi
if [[ -z $transcript || ! -f $transcript ]]; then
  echo "thread-size: no transcript found; check the triggers by hand" >&2
  exit 0
fi

# The context of the last model call is what every next call re-reads.
# jq does the date arithmetic, so GNU and BSD date differ in nothing here.
read -r start hours calls context < <(jq -rs '
  [.[] | select(.type == "assistant" and .message.usage != null)] as $a
  | ($a | map(.message.id) | unique | length) as $calls
  | ($a | last | .message.usage) as $u
  | ([.[] | .timestamp // empty] | first) as $t
  | "\($t) \((now - ($t | sub("\\.[0-9]+"; "") | fromdate)) / 3600 | floor) \($calls) \(($u.input_tokens // 0) + ($u.cache_read_input_tokens // 0) + ($u.cache_creation_input_tokens // 0))"
' "$transcript")

if [[ -z $merged ]]; then
  branch=$(git branch --show-current 2>/dev/null || true)
  repo=$(git remote get-url origin 2>/dev/null | sed -E 's#.*github.com[:/]##; s#\.git$##; s#^git/##')
  merged=$(gh api -X GET search/issues -f q="repo:$repo is:pr is:merged head:$branch merged:>=${start%%T*}" \
    --jq .total_count 2>/dev/null || echo unknown)
fi

echo "thread-size: $calls model calls, context $context tokens, $merged PRs merged from this branch, ${hours}h since $start"
over=()
((context >= max_context)) && over+=("context ≥ $max_context")
[[ $merged =~ ^[0-9]+$ ]] && ((merged >= max_merged)) && over+=("≥ $max_merged merged PRs")
((hours >= max_hours)) && over+=("≥ ${max_hours}h")
if ((${#over[@]})); then
  reasons=$(IFS=,; echo "${over[*]}")
  echo "thread-size: hand off now ($reasons): skill finish-task, \"Thread size and handoff\"" >&2
  exit 1
fi
