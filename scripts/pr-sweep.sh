#!/usr/bin/env bash
# Finds open pull requests that sit unfinished and says so on the PR (docs/runbook.md, "Idle
# pull requests"). No model, no secrets: `gh` with the workflow's own token.
#
# A PR is flagged when it
#   - conflicts with the base branch (it gets no CI, so nobody notices), or
#   - has no checks at all after 15 minutes (CI did not start), or
#   - has no commit for more than 2 hours (IDLE_HOURS).
# A flagged PR gets one comment per 24 hours, and the list goes to the job summary. Titles and
# comment bodies are data: they are only printed, never executed.
set -euo pipefail

IDLE_HOURS=${IDLE_HOURS:-2}
NOW=${PR_SWEEP_NOW:-$(date +%s)}
MARK='<!-- pr-sweep -->'
summary=${GITHUB_STEP_SUMMARY:-/dev/null}

epoch() { date -d "$1" +%s; }

prs=$(gh pr list --state open --limit 100 \
  --json number,title,isDraft,createdAt,mergeable,statusCheckRollup,commits)

flagged=0
{
  echo "## Open pull requests that need a decision"
  echo
} >>"$summary"

while read -r row; do
  n=$(jq -r .number <<<"$row")
  title=$(jq -r .title <<<"$row")
  created=$(epoch "$(jq -r .createdAt <<<"$row")")
  last=$(jq -r '[.commits[].committedDate] | max // empty' <<<"$row")
  last=$(epoch "${last:-$(jq -r .createdAt <<<"$row")}")
  reasons=()
  [[ $(jq -r .mergeable <<<"$row") == CONFLICTING ]] &&
    reasons+=("it conflicts with the base branch, so CI does not run: merge the base into the branch")
  if [[ $(jq '.statusCheckRollup | length' <<<"$row") -eq 0 ]] && ((NOW - created > 900)); then
    reasons+=("it has no checks: CI did not start")
  fi
  if ((NOW - last > IDLE_HOURS * 3600)); then
    reasons+=("no commit for more than $IDLE_HOURS hours: merge it, or close it with a reason")
  fi
  ((${#reasons[@]})) || continue
  flagged=$((flagged + 1))
  draft=""
  [[ $(jq -r .isDraft <<<"$row") == true ]] && draft=" (draft)"
  printf -- '- #%s%s %s: %s\n' "$n" "$draft" "$title" "$(IFS=';'; echo "${reasons[*]}")" >>"$summary"

  # One comment per day: the newest marker comment decides.
  prev=$(gh pr view "$n" --json comments \
    --jq "[.comments[] | select(.body | contains(\"$MARK\")) | .createdAt] | max // empty")
  if [[ -n $prev ]] && ((NOW - $(epoch "$prev") < 86400)); then
    continue
  fi
  body="$MARK
This pull request needs a decision:"
  for r in "${reasons[@]}"; do body+=$'\n'"- $r"; done
  body+=$'\n\n'"Open pull requests are unfinished work (AGENTS.md, \"How to work with GitHub\"). _Posted by \`scripts/pr-sweep.sh\`._"
  gh pr comment "$n" --body "$body" >/dev/null
done < <(jq -c '.[]' <<<"$prs")

((flagged)) || echo "None." >>"$summary"
echo "pr-sweep: $flagged open pull request(s) flagged"
