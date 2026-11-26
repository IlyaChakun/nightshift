#!/usr/bin/env bash
# Run with the verified local PR head. Merge mode is an explicit decision by the caller.
set -euo pipefail

die() { echo "pr-gate: $*" >&2; exit 1; }
usage() { die 'usage: scripts/pr-gate.sh [--git-auth] preflight | wait <PR> <verified-head> | merge <PR> <verified-head>'; }

origin=$(git remote get-url origin)
token=
if [[ ${1:-} == --git-auth ]]; then
  shift
  [[ $origin =~ ^https://github\.com/[[:alnum:]_.-]+/[[:alnum:]_.-]+$ ]] || die '--git-auth requires a canonical HTTPS origin on GitHub.com'
  # Scope the existing Git credential to this script. Keep credential values out of traces.
  set +x
  credential=$(printf 'url=%s\n\n' "$origin" | GIT_TERMINAL_PROMPT=0 git credential fill)
  token=$(printf '%s\n' "$credential" | sed -n 's/^password=//p')
  unset credential
  [[ -n $token ]] || die 'no credential for origin'
fi
github() {
  if [[ -n $token ]]; then
    GH_TOKEN="$token" gh "$@"
  else
    gh "$@"
  fi
}

mode=${1:-}
case "$mode" in
  preflight) [[ $# == 1 ]] || usage ;;
  wait|merge) [[ $# == 3 && $2 =~ ^[0-9]+$ && $3 =~ ^[0-9a-f]{40}$ ]] || usage ;;
  *) usage ;;
esac
repo_info=$(github repo view "$origin" --json nameWithOwner,url,viewerPermission)
repo=$(jq -er '.nameWithOwner' <<<"$repo_info")
host=$(jq -er '.url | split("/")[2]' <<<"$repo_info")
permission=$(jq -er '.viewerPermission' <<<"$repo_info")
case "$permission" in
  WRITE|MAINTAIN|ADMIN) ;;
  *) die "$repo needs a writable GitHub session (current permission: $permission); retry with --git-auth if origin has a credential" ;;
esac
if [[ $mode == preflight ]]; then
  echo "$repo: $permission"
  exit 0
fi

pr=$2
head=$3
branch=$(git symbolic-ref --quiet --short HEAD)
[[ $(git rev-parse HEAD) == "$head" ]] || die 'local HEAD differs from the verified head'
worktree_status=$(git status --porcelain)
[[ -z $worktree_status ]] || die 'worktree is dirty'
git fetch origin main
base=$(git rev-parse origin/main)
git merge-base --is-ancestor "$base" "$head" || die 'verified head lacks latest main'
remote_repo="$host/$repo"

needs_codeql=false
changed_files=$(git diff --no-renames --name-only "$base" "$head")
while IFS= read -r file; do
  case "$file" in
    docs/*|.agents/*) ;;
    *) [[ $file == *.md && $file != */* ]] || needs_codeql=true ;;
  esac
done <<<"$changed_files"

read_pr() {
  data=$(github pr view "$pr" --repo "$remote_repo" --json author,baseRefName,headRefName,headRefOid,isCrossRepository,isDraft,mergeStateStatus,state,statusCheckRollup)
  jq -e --arg head "$head" --arg branch "$branch" '
    .headRefOid == $head and .headRefName == $branch and .baseRefName == "main"
    and .state == "OPEN" and (.isCrossRepository or .isDraft | not)
  ' <<<"$data" >/dev/null || die 'PR is not a ready branch in this checkout at the verified head'
  case $(jq -r '.mergeStateStatus' <<<"$data") in
    BEHIND|DIRTY) die 'PR needs current main or has a merge conflict' ;;
  esac
}

pr_checks() {
  jq -r --argjson codeql "$needs_codeql" '
    .statusCheckRollup as $checks
    | if any($checks[]; (.conclusion // .state) | IN("ACTION_REQUIRED","CANCELLED","ERROR","FAILURE","STALE","STARTUP_FAILURE","TIMED_OUT"))
      then "failed"
      elif any($checks[]; .status != null and .status != "COMPLETED" or .state == "PENDING")
      then "pending"
      else [$checks[] | select(.conclusion == "SUCCESS") | [.workflowName,.name]] as $green
      | (if $codeql then [["verify","ci-ok"],["codeql","analyze (go)"],["codeql","analyze (actions)"]] else [["verify","ci-ok"]] end) as $required
      | if all($required[]; . as $want | any($green[]; . == $want))
        then "passed" else "missing" end
      end
  ' <<<"$data"
}

workflow_checks() {
  local commit=$1 on_branch=$2 event=$3 codeql=$4 runs workflow result overall=passed
  local workflows=(verify)
  if [[ $codeql == true ]]; then workflows+=(codeql); fi
  for workflow in "${workflows[@]}"; do
    runs=$(github run list --repo "$remote_repo" --workflow ".github/workflows/$workflow.yml" \
      --branch "$on_branch" --commit "$commit" --event "$event" --json headSha,status,conclusion) ||
      die "cannot read $workflow on $on_branch at $commit"
    result=$(jq -r --arg sha "$commit" '
      first(.[] | select(.headSha == $sha)) // {}
      | if .status != "completed" then "pending"
        elif .conclusion == "success" then "passed" else "failed" end
    ' <<<"$runs") || die "cannot parse $workflow on $on_branch at $commit"
    case "$result" in
      failed) echo failed; return ;;
      pending) overall=pending ;;
      passed) ;;
      *) die "unexpected $workflow status at $commit" ;;
    esac
  done
  echo "$overall"
}

wait_main() {
  local commit=$1 deadline=$((SECONDS + 600)) result
  while ((SECONDS < deadline)); do
    result=$(workflow_checks "$commit" main push true)
    case "$result" in
      passed) echo "main $commit: Verify and CodeQL passed"; return ;;
      failed) die "main $commit: Verify or CodeQL failed" ;;
    esac
    sleep 15
  done
  die "main $commit: timed out waiting for Verify and CodeQL"
}

deadline=$((SECONDS + 600))
checks_deadline=$((SECONDS + 60))
while ((SECONDS < deadline)); do
  read_pr
  result=$(pr_checks)
  if [[ $result == passed ]]; then
    result=$(workflow_checks "$head" "$branch" pull_request "$needs_codeql")
  fi
  case "$result" in
    passed) break ;;
    failed) die "PR #$pr: a check failed" ;;
    missing) ((SECONDS < checks_deadline)) || die "PR #$pr: required checks are missing" ;;
  esac
  sleep 15
done
[[ ${result:-} == passed ]] || die "PR #$pr: timed out waiting for required checks"
echo "PR #$pr at $head: checks passed"
[[ $mode == merge ]] || exit 0

actor=$(github api --hostname "$host" user --jq .login)
jq -e --arg actor "$actor" '.author.login == $actor' <<<"$data" >/dev/null || die 'PR author differs from the GitHub session'
wait_main "$base"
if [[ $needs_codeql == true ]]; then
  merge_ref="refs%2Fpull%2F$pr%2Fmerge"
  merge_head=$(git ls-remote origin "refs/pull/$pr/merge" | cut -f1)
  [[ $merge_head =~ ^[0-9a-f]{40}$ ]] || die 'missing synthetic PR merge commit'
  parents=$(github api --hostname "$host" "repos/$repo/commits/$merge_head" --jq '[.parents[].sha]')
  jq -e --arg base "$base" --arg head "$head" '. == [$base,$head]' <<<"$parents" >/dev/null ||
    die 'synthetic PR merge commit does not match latest main and verified head'
  analyses=$(github api --hostname "$host" --paginate --slurp "repos/$repo/code-scanning/analyses?ref=$merge_ref")
  jq -e --arg sha "$merge_head" '
    [ .[][] | select(.commit_sha == $sha and .analysis_key == ".github/workflows/codeql.yml:analyze" and .error == "") | .category ] as $categories
    | all(["/language:go","/language:actions"][]; . as $want | any($categories[]; . == $want))
  ' <<<"$analyses" >/dev/null || die 'missing CodeQL analyses for the current PR merge commit'
  alerts() {
    github api --hostname "$host" --paginate "repos/$repo/code-scanning/alerts?state=open&ref=$1" --jq '.[].number'
  }
  old_alerts=$(alerts refs%2Fheads%2Fmain | LC_ALL=C sort -u) || die 'cannot read or sort main alerts'
  pr_alerts=$(alerts "$merge_ref" | LC_ALL=C sort -u) || die 'cannot read or sort PR alerts'
  new_alerts=$(LC_ALL=C comm -13 <(printf '%s\n' "$old_alerts") <(printf '%s\n' "$pr_alerts"))
  [[ -z $new_alerts ]] || die "new open CodeQL alerts: $new_alerts"
fi
read_pr
[[ $(pr_checks) == passed && $(jq -r '.mergeStateStatus' <<<"$data") == CLEAN ]] || die 'PR is no longer green and mergeable'
rules=$(github api --hostname "$host" "repos/$repo/rules/branches/main")
jq -e 'any(.[]; .type == "required_status_checks" and .parameters.strict_required_status_checks_policy
  and any(.parameters.required_status_checks[]; .context == "ci-ok"))' <<<"$rules" >/dev/null ||
  die 'main lacks the required up-to-date ci-ok rule'
[[ $(git ls-remote origin refs/heads/main | cut -f1) == "$base" ]] || die 'main moved; verify a head that contains it'
merge_endpoint="repos/$repo/pulls/$pr/merge-async"
merge_api() {
  github api --hostname "$host" -H 'X-GitHub-Api-Version: 2026-03-10' "$@"
}
response=$(merge_api -X PUT "$merge_endpoint" -f "sha=$head" -f merge_method=squash -f merge_action=direct_merge -F bypass_rules=false)
merge_deadline=$((SECONDS + 120))
while [[ $(jq -r '.status' <<<"$response") == pending ]] && ((SECONDS < merge_deadline)); do
  request=$(jq -er '.details.uuid' <<<"$response")
  [[ $request =~ ^[0-9a-f-]{36}$ ]] || die 'invalid asynchronous merge request identifier'
  sleep 5
  response=$(merge_api "$merge_endpoint/$request")
done
commit=$(jq -er 'select(.status == "merged") | .details.sha' <<<"$response") ||
  die "PR #$pr: merge not confirmed: $(jq -r '.details.message // .status' <<<"$response")"
[[ $commit =~ ^[0-9a-f]{40}$ ]] || die "PR #$pr: invalid merged SHA"
echo "PR #$pr merged as $commit"
wait_main "$commit"
