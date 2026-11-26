#!/usr/bin/env bash
# Offline merge-gate scenarios. The stub only records a merge request.
set -euo pipefail
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"
export GATE_HEAD=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
export GATE_BASE=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
export GATE_MERGE=cccccccccccccccccccccccccccccccccccccccc
export GATE_TRACE="$tmp/trace"

cat > "$tmp/bin/git" <<'STUB'
#!/usr/bin/env bash
[[ ${GH_TOKEN:-} != fixture-credential ]] || exit 9
case "$*" in
  "remote get-url origin") echo https://github.com/example/repo.git ;;
  "credential fill") printf 'password=fixture-credential\n' ;;
  "symbolic-ref --quiet --short HEAD") echo codex/example ;;
  "rev-parse HEAD") echo "$GATE_HEAD" ;;
  "rev-parse origin/main") echo "$GATE_BASE" ;;
  "status --porcelain") [[ ${GATE_STATUS_FAIL:-false} != true ]] || { echo 'worktree lookup failed' >&2; exit 9; } ;;
  "fetch origin main"|"merge-base --is-ancestor "*) ;;
  "diff --no-renames --name-only "*) echo "${GATE_FILE:-internal/code.go}" ;;
  "ls-remote origin refs/heads/main") printf '%s\trefs/heads/main\n' "${GATE_LATEST:-$GATE_BASE}" ;;
  "ls-remote origin refs/pull/12/merge") printf '%s\trefs/pull/12/merge\n' "$GATE_MERGE" ;;
  *) exit 2 ;;
esac
STUB

cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GATE_TRACE"
case "$1 $2" in
  "repo view")
    permission=${GATE_PERMISSION:-WRITE}
    if [[ ${GH_TOKEN:-} == fixture-credential ]]; then permission=WRITE; fi
    jq -n --arg permission "$permission" '{nameWithOwner:"example/repo",url:"https://github.com/example/repo",viewerPermission:$permission}' ;;
  "pr view")
    if [[ ${*: -1} == state,mergeCommit ]]; then
      jq -n --arg sha "$GATE_MERGE" '{state:"MERGED",mergeCommit:{oid:$sha}}'
    else
      jq -n --arg head "${GATE_REMOTE_HEAD:-$GATE_HEAD}" --arg conclusion "${GATE_CHECK:-SUCCESS}" \
        --arg name "${GATE_CHECK_NAME:-ci-ok}" --argjson codeql "${GATE_CODEQL:-true}" '
        {headRefOid:$head,headRefName:"codex/example",baseRefName:"main",state:"OPEN",isDraft:false,isCrossRepository:false,
        mergeStateStatus:"CLEAN",author:{login:"owner"},statusCheckRollup:(
          [ {name:$name,workflowName:"verify",status:"COMPLETED",conclusion:$conclusion} ]
          + if $codeql then [ "analyze (go)", "analyze (actions)" ] | map({name:.,workflowName:"codeql",status:"COMPLETED",conclusion:"SUCCESS"}) else [] end
        )}'
    fi ;;
  "run list")
    while [[ $1 != --workflow ]]; do shift; done
    workflow=$2
    while [[ $1 != --commit ]]; do shift; done
    sha=$2
    while [[ $1 != --event ]]; do shift; done
    [[ ${GATE_LOOKUP_FAIL:-} != "$2" ]] || exit 9
    if [[ ${GATE_INVALID_JSON:-false} == true ]]; then echo 'bad json'; exit 0; fi
    result=${GATE_PR_RUN:-success}
    if [[ $2 == push ]]; then result=${GATE_MAIN:-success}; fi
    if [[ $workflow == .github/workflows/codeql.yml ]]; then result=success; fi
    jq -n --arg sha "$sha" --arg conclusion "$result" '[{headSha:$sha,status:"completed",conclusion:$conclusion}]' ;;
  "api --hostname")
    case "$*" in
      *" user "*) echo "${GATE_ACTOR:-owner}" ;;
      */branches/main) echo '[{"type":"required_status_checks","parameters":{"strict_required_status_checks_policy":true,"required_status_checks":[{"context":"ci-ok"}]}}]' ;;
      */commits/*) jq -cn --arg base "${GATE_SYNTHETIC_BASE:-$GATE_BASE}" --arg head "$GATE_HEAD" '[$base,$head]' ;;
      *code-scanning/analyses*) jq -cn --arg sha "${GATE_ANALYSIS_HEAD:-$GATE_MERGE}" '
        [[ ["/language:go","/language:actions"][] | {commit_sha:$sha,analysis_key:".github/workflows/codeql.yml:analyze",category:.,error:""} ]]' ;;
      *refs%2Fheads%2Fmain*) echo 7 ;;
      *refs%2Fpull*) [[ ${GATE_SCAN_DENIED:-false} != true ]] || { echo 'CodeQL access denied' >&2; exit 1; }; echo "${GATE_ALERT:-7}" ;;
      *" -X PUT "*" -F bypass_rules=false")
        if [[ ${GATE_ASYNC:-false} == true ]]; then
          echo '{"status":"pending","details":{"uuid":"11111111-1111-1111-1111-111111111111"}}'
        else
          jq -cn --arg sha "$GATE_MERGE" '{status:"merged",details:{sha:$sha}}'
        fi ;;
      */merge-async/11111111-1111-1111-1111-111111111111)
        jq -cn --arg sha "$GATE_MERGE" '{status:"merged",details:{sha:$sha}}' ;;
      *) exit 2 ;;
    esac ;;
  *) exit 2 ;;
esac
STUB
cat > "$tmp/bin/sleep" <<'STUB'
#!/usr/bin/env bash
[[ ${GATE_ASYNC:-false} != true ]] || exit 0
echo 'waiting without the exact required check' >&2
exit 1
STUB
cat > "$tmp/bin/sort" <<'STUB'
#!/usr/bin/env bash
[[ ${GATE_SORT_FAIL:-false} != true ]] || exit 9
exec /usr/bin/sort "$@"
STUB
chmod +x "$tmp/bin/"*
export PATH="$tmp/bin:$PATH"

reject() {
  local reason=$1
  shift
  : > "$GATE_TRACE"
  if env "$@" scripts/pr-gate.sh merge 12 "$GATE_HEAD" > "$tmp/output" 2>&1; then
    echo "test-pr-gate: accepted $reason" >&2
    exit 1
  fi
  grep -F "$reason" "$tmp/output" >/dev/null || { cat "$tmp/output" "$GATE_TRACE" >&2; exit 1; }
  if grep -q -- ' -X PUT ' "$GATE_TRACE"; then exit 1; fi
}
reject 'current permission: READ' GATE_PERMISSION=READ
reject 'verified head' GATE_REMOTE_HEAD="$GATE_BASE"
reject 'worktree lookup failed' GATE_STATUS_FAIL=true
reject 'a check failed' GATE_CHECK=FAILURE
reject 'a check failed' GATE_PR_RUN=failure
reject 'cannot read verify on codex/example' GATE_LOOKUP_FAIL=pull_request
reject 'cannot read verify on main' GATE_LOOKUP_FAIL=push
reject 'cannot parse verify' GATE_INVALID_JSON=true
reject 'waiting without the exact required check' GATE_CHECK_NAME=supplemental-ci-ok
reject 'Verify or CodeQL failed' GATE_MAIN=failure
reject 'PR author differs' GATE_ACTOR=another
reject 'CodeQL access denied' GATE_SCAN_DENIED=true
reject 'cannot read or sort main alerts' GATE_SORT_FAIL=true
reject 'does not match latest main and verified head' GATE_SYNTHETIC_BASE="$GATE_MERGE"
reject 'missing CodeQL analyses' GATE_ANALYSIS_HEAD="$GATE_HEAD"
reject 'new open CodeQL alerts' GATE_ALERT=9
reject 'main moved' GATE_LATEST="$GATE_MERGE"

: > "$GATE_TRACE"
GATE_FILE=docs/example.md GATE_CODEQL=false scripts/pr-gate.sh wait 12 "$GATE_HEAD" >/dev/null
if grep -q -- ' -X PUT ' "$GATE_TRACE"; then exit 1; fi

GATE_PERMISSION=READ GATE_ASYNC=true scripts/pr-gate.sh --git-auth merge 12 "$GATE_HEAD" > "$tmp/output"
grep -F -- " -X PUT repos/example/repo/pulls/12/merge-async -f sha=$GATE_HEAD -f merge_method=squash -f merge_action=direct_merge -F bypass_rules=false" "$GATE_TRACE" >/dev/null
grep -Fx "main $GATE_MERGE: Verify and CodeQL passed" "$tmp/output" >/dev/null
echo 'test-pr-gate: ok'
