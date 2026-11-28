#!/usr/bin/env bash
# Checks a commit of main on this machine and moves the stable instance to it
# (docs/runbook.md, "Upgrade"). Nothing is tagged or upgraded unless every check passes.
#
#   deploy/upgrade.sh [--check-only] [commit]     default commit: origin/main
#
# It runs, in order: CI of the commit, the drill, then on the development stack both doctors,
# a tool install from an empty directory, a module download through modcache, a sandbox task
# that must reach `released`, the off-site copy, the restore test and the logs. Then it tags
# the commit, runs scripts/stable.sh up, reads the logs of the new release and moves CLI state
# from before per-tenant HOMEs out of the agent's reach (deploy/legacy-home.sh).
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
state=${NIGHTSHIFT_STABLE_HOME:-$HOME/.nightshift/stable}
tenant=${NIGHTSHIFT_CHECK_TENANT:-example-solo}
dev_project=${NIGHTSHIFT_CHECK_PROJECT:-deploy}
task_timeout=${NIGHTSHIFT_CHECK_TASK_SECONDS:-600}
idle_timeout=${NIGHTSHIFT_CHECK_IDLE_SECONDS:-900}
ci_timeout=${NIGHTSHIFT_CHECK_CI_SECONDS:-900}
settle=${NIGHTSHIFT_CHECK_SETTLE_SECONDS:-60}
poll=${NIGHTSHIFT_CHECK_POLL_SECONDS:-10}
dev_preview_subnet=${NIGHTSHIFT_CHECK_PREVIEW_SUBNET:-172.31.253.0/24}
dev_deploy_dir=${NIGHTSHIFT_CHECK_DEPLOY_DIR:-${NIGHTSHIFT_DEPLOY_DIR:-/tmp/nightshift-deploy}-check}
work=""
dev_started=0
stable_paused=0
unverified="" # the previous tag, while a new release is not yet seen to work

die() {
  echo "upgrade: $*" >&2
  exit 1
}

step() { echo "upgrade: $*"; }

stable() { "$repo/scripts/stable.sh" "$@"; }

# dev <args>...: docker compose on the development stack, built from the checked commit with
# this checkout's config and secrets. Never the obs profile: the stable instance holds its port.
# The deploy profile is on, as on the stable instance, so a tenant with compose-local gets its
# preview; its preview network, checkouts and environment names are not the stable instance's,
# which runs beside it on the same engine.
dev() {
  NIGHTSHIFT_CONFIG=$repo/deploy/config NIGHTSHIFT_OFFSITE=$repo/deploy/offsite NIGHTSHIFT_OTLP="" NIGHTSHIFT_TELEMETRY=0 \
    NIGHTSHIFT_PREVIEW_SUBNET=$dev_preview_subnet NIGHTSHIFT_DEPLOY_DIR=$dev_deploy_dir NIGHTSHIFT_ENV_PREFIX=nscheck \
    docker compose -p "$dev_project" --env-file "$repo/deploy/.env" -f "$work/dev/deploy/docker-compose.yml" --profile deploy "$@"
}

# book <compose function> <sql>: one value from an instance's book.
book() { "$1" exec -T postgres psql -U postgres -d book -tA -c "$2"; }
stable_compose() { stable compose "$@"; }

cleanup() {
  local status=$?
  trap '' INT TERM HUP # a second interrupt must not leave the stable worker stopped
  set +e
  if [[ -n $unverified ]]; then
    echo "upgrade: the new release is not verified; to go back: scripts/stable.sh up $unverified" >&2
  fi
  # One subscription session at a time: the stable worker stays down while the development
  # stack may still run a job.
  # After a failure the containers are only stopped, so their logs stay for a look.
  if ((dev_started)) && ! dev stop >/dev/null 2>&1; then
    echo "upgrade: the development stack did not stop; stop it, then: scripts/stable.sh compose start worker" >&2
    stable_paused=0
  fi
  if ((stable_paused)); then
    stable compose start worker >/dev/null 2>&1 || echo "upgrade: start the stable worker by hand: scripts/stable.sh compose start worker" >&2
  fi
  if [[ -n $work ]]; then rm -rf "$work"; fi
  exit "$status"
}

# fill_secrets <deploy dir> <release>: a release may add a generated secret. init-secrets.sh creates only
# the missing files and keeps the others; on Linux it needs root to set their owners.
fill_secrets() {
  local f missing=0
  [[ -f $1/docker-compose.yml ]] || return 0
  for f in $(sed -nE 's#.*file: \./secrets/([^ }]+).*#\1#p' "$1/docker-compose.yml"); do
    [[ -e $1/secrets/$f ]] || missing=1
  done
  ((missing)) || return 0
  "$1/init-secrets.sh" >/dev/null || die "secrets are missing; run as root the deploy/init-secrets.sh of $2"
}

# export_commit <directory>: the sources of the commit, without touching this checkout.
export_commit() {
  mkdir -p "$1"
  git -C "$repo" archive "$commit" | tar -x -C "$1"
}

# verifier_provider fails early when the check tenant names no verifier provider. Without one the
# factory fails closed: the sandbox task stops in human_review with verifier_unavailable, and the
# check would wait for a release that never comes (docs/runbook.md, "Quiet hours and review waits").
verifier_provider() {
  local dir file
  # instance.yaml names the tenants directory, relative to the config directory.
  dir=$(sed -nE "s/^tenants:[[:space:]]*['\"]?([^'\" #]+)['\"]?.*/\1/p" "$repo/deploy/config/instance.yaml")
  file=$repo/deploy/config/${dir:-tenants}
  file=${file%/}/$tenant/tenant.yaml
  [[ -f $file ]] || die "no $file; set NIGHTSHIFT_CHECK_TENANT to a tenant of deploy/config"
  # providers: { ..., verifier: [x] } or a block with verifier: [x] or a "- x" list under it.
  awk -v q="'" '
    { sub(/(^|[ \t])#.*/, ""); gsub(/"/, ""); gsub(q, "") }
    flow { buf = buf " " $0; if (index($0, "}")) { flow = 0; if (buf ~ /verifier:[ \t]*\[[ \t]*[A-Za-z0-9_]/) found = 1 } ; next }
    /^providers:/ {
      rest = substr($0, 11)
      if (index(rest, "{")) { buf = rest; if (index(rest, "}")) { if (buf ~ /verifier:[ \t]*\[[ \t]*[A-Za-z0-9_]/) found = 1 } else flow = 1 }
      else block = 1
      next
    }
    block && /^[^ \t]/ && NF { block = 0; list = 0 }
    block && NF { i = match($0, /[^ \t]/) - 1; if (!ind) ind = i }
    block && list && NF && i >= ind && /^[ \t]*-[ \t]*[A-Za-z0-9_]/ { found = 1; next }
    block && NF && i == ind { list = 0 }
    block && NF && i == ind && /^[ \t]+verifier:/ { v = $0; sub(/^[ \t]+verifier:[ \t]*/, "", v); if (v ~ /^\[[ \t]*[A-Za-z0-9_]/) found = 1; list = (v == ""); next }
    END { exit !found }
  ' "$file" || die "tenant $tenant names no verifier provider in $file (providers.verifier); without one the sandbox task stops in human_review with verifier_unavailable"
}

# ci_cancelled stops the wait when every verify run of the commit was cancelled: a newer push to
# main replaced a queued run, so ci-ok never starts and waiting would only burn ci_timeout.
ci_cancelled() {
  local runs
  runs=$(cd "$repo" && gh api "repos/{owner}/{repo}/actions/runs?head_sha=$commit" \
    --jq '[.workflow_runs[] | select(.name == "verify") | "\(.status) \(.conclusion)"] | unique | join(",")') || die "cannot read the runs of $commit"
  [[ $runs == "completed cancelled" ]] &&
    die "the verify run of $commit was cancelled before ci-ok ran (a newer push replaced it); check a newer commit: deploy/upgrade.sh <commit>"
  return 0
}

# ci_green waits for the ci-ok check of the commit and requires its success.
ci_green() {
  local result waited=0
  while :; do
    # gh takes {owner}/{repo} from the directory it runs in.
    result=$(cd "$repo" && gh api "repos/{owner}/{repo}/commits/$commit/check-runs?check_name=ci-ok" \
      --jq '.check_runs[0] | "\(.status) \(.conclusion)"') || die "cannot read CI of $commit"
    case $result in
    "completed success") return 0 ;;
    completed*) die "ci-ok of $commit is ${result#completed }" ;;
    "null null") ci_cancelled ;; # no ci-ok yet: it may be coming, or the whole run may be gone
    esac
    ((waited < ci_timeout)) || die "ci-ok of $commit did not finish in ${ci_timeout}s"
    sleep "$poll"
    waited=$((waited + poll + 1))
  done
}

# A subscription allows one session at a time, so the stable worker rests during the check.
# A job may be claimed between the count and the stop; then the worker goes back to work.
pause_stable() {
  local busy running waited=0
  while :; do
    busy=$(book stable_compose "SELECT count(*) FROM book.jobs WHERE state IN ('queued', 'running')") || die "cannot read the stable book"
    if [[ $busy == 0 ]]; then
      stable_paused=1
      stable compose stop worker
      running=$(book stable_compose "SELECT count(*) FROM book.jobs WHERE state = 'running'") || die "cannot read the stable book"
      [[ $running == 0 ]] && return 0
      stable compose start worker
      stable_paused=0
    fi
    ((waited < idle_timeout)) || die "the stable instance is still busy after ${idle_timeout}s"
    sleep "$poll"
    waited=$((waited + poll + 1))
  done
}

# The development stack checks the agent CLIs that the stable instance will be built with.
same_versions() {
  local a b
  [[ -f $state/env ]] || return 0
  # A file may pin neither CLI; grep then finds nothing, which is not an error.
  a=$(grep -E '^(CLAUDE_CODE|CODEX)_VERSION=' "$repo/deploy/.env" | sort || true)
  b=$(grep -E '^(CLAUDE_CODE|CODEX)_VERSION=' "$state/env" | sort || true)
  [[ $a == "$b" ]] || die "deploy/.env and $state/env pin different agent CLI versions"
}

wait_healthy() {
  local i
  for ((i = 0; i < 60; i++)); do
    dev exec -T nightshift nightshift status --health >/dev/null 2>&1 && return 0
    sleep 2
  done
  die "the development stack is not healthy"
}

tool_install() {
  [[ -f $work/dev/mise.toml ]] || return 0
  # shellcheck disable=SC2016 # expanded by the shell in the container
  tar -C "$work/dev" -cf - mise.toml mise.lock .mise | dev exec -T -u 20001 toolchain sh -c '
    d=/work/_upgradecheck; rm -rf "$d" && mkdir -p "$d/p" && cd "$d/p" && tar -xf - &&
    HOME=/home/agent MISE_DATA_DIR="$d/data" MISE_CACHE_DIR="$d/cache" MISE_TRUSTED_CONFIG_PATHS="$d/p" MISE_YES=1 mise install --yes
    status=$?; cd /; rm -rf "$d"; exit $status'
}

# The module is removed from the cache first, so that it has to come from the public proxy.
module_download() {
  dev exec -T modcache rm -rf /var/lib/athens/github.com/klauspost/compress/v1.18.0 || die "cannot clear the module from the cache"
  dev exec -T -u 20001 worker sh -c '
    curl -fsS --max-time 180 -o /dev/null http://modcache:3000/github.com/klauspost/compress/@v/v1.18.0.zip &&
    curl -fsS --max-time 60 -o /dev/null http://modcache:3000/sumdb/sum.golang.org/supported'
}

sandbox_task() {
  local short=${commit:0:7} name added id state="" waited=0
  name=live-$short-$(date +%H%M%S)
  added=$(dev exec -T nightshift nightshift task add "$tenant" "feat: add $name.sh that prints exactly: live check $short") ||
    die "cannot add the sandbox task"
  echo "$added"
  id=$(printf '%s\n' "$added" | sed -n 's/^queued task #\([0-9][0-9]*\).*/\1/p')
  [[ -n $id ]] || die "task add did not name the task"
  while ((waited < task_timeout)); do
    state=$(book dev "SELECT state FROM book.tasks WHERE id = $id")
    case $state in
    released) return 0 ;;
    escalated | human_review | canceled) die "sandbox task #$id is in $state, not released" ;;
    esac
    sleep "$poll"
    waited=$((waited + poll + 1))
  done
  die "sandbox task #$id is still in '$state' after ${task_timeout}s"
}

# errors_in <compose function> <since>: error lines of every service. A worker that asks for a
# job while nightshift is still starting is not one.
errors_in() {
  local logs
  logs=$("$1" logs --since "$2" 2>&1) || die "cannot read the logs"
  printf '%s\n' "$logs" |
    grep -E '"level":"(ERROR|error|fatal|panic)"| (ERROR|FATAL|PANIC):|level=(error|fatal)|\| (panic:|fatal error:)' |
    grep -vE 'jobs/claim.*connection refused' || true
}

check() {
  local since found
  step "CI"
  ci_green
  step "drill"
  export_commit "$work/drill" # its own random secrets, never this checkout's
  "$work/drill/scripts/drill.sh"
  step "pause the stable worker"
  pause_stable
  step "development stack at ${commit:0:7}"
  export_commit "$work/dev"
  ln -sfn "$repo/deploy/secrets" "$work/dev/deploy/secrets"
  fill_secrets "$work/dev/deploy" "${commit:0:7}"
  since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  dev_started=1
  dev up -d --build --remove-orphans
  wait_healthy
  step "doctor"
  dev exec -T nightshift nightshift doctor
  dev exec -T worker nightshift doctor --worker
  step "tool install from an empty directory"
  tool_install
  step "module download through modcache"
  module_download
  step "sandbox task"
  sandbox_task
  step "backup, off-site copy and restore test"
  dev exec -T backup /etc/backup/dump.sh
  dev exec -T offsite sh /etc/backup/offsite.sh
  dev exec -T backup /etc/backup/restore-test.sh
  step "logs"
  found=$(errors_in dev "$since")
  [[ -z $found ]] || die "errors in the logs of the development stack:
$(printf '%s\n' "$found" | head -n 10)"
  step "stop the development stack"
  dev down || die "the development stack did not stop"
  dev_started=0
}

next_tag() {
  local base n=2 name
  base=factory-stable-$(date +%F)
  name=$base
  while git -C "$repo" rev-parse -q --verify "refs/tags/$name" >/dev/null; do
    name=$base.$n
    n=$((n + 1))
  done
  echo "$name"
}

# stable up checks the cached commit of the tag and runs both doctors. Confirm that the
# running services actually came from that release's Compose project file as well.
running_release() {
  local tag=$1 service id config
  [[ $(stable current) == "$tag" ]] || return 1
  for service in nightshift worker; do
    id=$(stable compose ps -q "$service") && [[ -n $id ]] || return 1
    config=$(docker inspect --format '{{if .State.Running}}{{index .Config.Labels "com.docker.compose.project.config_files"}}{{end}}' "$id") || return 1
    [[ $config == "$state/releases/$tag/deploy/docker-compose.yml" ]] || return 1
  done
}

restore_previous() {
  local previous=$1 candidate=$2 reason=$3
  step "$reason; going back to $previous"
  if ! stable up "$previous" || ! running_release "$previous"; then
    die "recovery could not be confirmed; run: scripts/stable.sh up $previous"
  fi
  unverified=""
  die "$previous is running again; the upgrade failed (ignore the launcher's hint to go back to $candidate)"
}

upgrade() {
  local tag previous since found services down
  previous=$(stable current)
  git -C "$repo" fetch -q --tags origin
  tag=$(next_tag)
  step "tag $tag"
  git -C "$repo" tag -a "$tag" "$commit" -m "Live check by deploy/upgrade.sh passed"
  if ! git -C "$repo" push -q origin "refs/tags/$tag"; then
    git -C "$repo" tag -d "$tag" >/dev/null
    die "cannot push the tag; nothing was upgraded"
  fi
  step "upgrade the stable instance, previous release $previous"
  since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  unverified=$previous
  stable_paused=0 # up can change the selected tag before returning, including on interruption
  stable up "$tag" || restore_previous "$previous" "$tag" "$tag did not start"
  sleep "$settle"
  # -a lists stopped services too. A one-shot (migrate, grafana-login) is meant to exit with 0; a
  # service with a restart policy is restarted after a 0 as well, so it never rests in "exited 0".
  services=$(stable compose ps -a --format '{{.Service}} {{.State}} {{.ExitCode}}') || restore_previous "$previous" "$tag" "cannot list the stable services"
  down=$(printf '%s\n' "$services" | grep -E ' (restarting|exited|dead) ' | grep -vE ' exited 0$' || true)
  [[ -z $down ]] || restore_previous "$previous" "$tag" "a service of $tag does not stay up:
$down"
  found=$(errors_in stable_compose "$since") || restore_previous "$previous" "$tag" "cannot read the stable logs"
  [[ -z $found ]] || restore_previous "$previous" "$tag" "$tag runs, but its logs hold errors:
$(printf '%s\n' "$found" | head -n 10)"
  unverified=""
  step "move CLI state from before per-tenant HOMEs out of the agent's reach"
  legacy_home "$tag"
  step "$tag is running"
}

# legacy_home moves the pre-#310 CLI state out of the stable instance's shared agent HOME
# (deploy/legacy-home.sh). The worker drops every capability, and the agent user owns that HOME, so
# a one-off container runs the script as root with the two capabilities a move into a root-only
# directory needs.
legacy_home() {
  stable compose run --rm --no-deps -T --user 0 --cap-add DAC_OVERRIDE --cap-add FOWNER --entrypoint sh worker -s -- /home/agent \
    <"$repo/deploy/legacy-home.sh" || die "$1 runs, but the old CLI state in /home/agent was not moved; retry only this step: scripts/stable.sh compose run --rm --no-deps -T --user 0 --cap-add DAC_OVERRIDE --cap-add FOWNER --entrypoint sh worker -s -- /home/agent <deploy/legacy-home.sh"
}

main() {
  local only=0 ref=origin/main
  if [[ ${1:-} == --check-only ]]; then
    only=1
    shift
  fi
  [[ $# -le 1 && ${1:-} != -* ]] || die "usage: deploy/upgrade.sh [--check-only] [commit]"
  git -C "$repo" fetch -q origin main || die "cannot fetch origin"
  commit=$(git -C "$repo" rev-parse -q --verify "${1:-$ref}^{commit}") || die "no commit ${1:-$ref}"
  git -C "$repo" merge-base --is-ancestor "$commit" origin/main || die "$commit is not on main"
  [[ -f $repo/deploy/config/instance.yaml && -f $repo/deploy/.env ]] || die "the development stack is not set up (docs/setup.md)"

  verifier_provider
  same_versions
  work=$(mktemp -d "${TMPDIR:-/tmp}/nightshift-upgrade.XXXXXX")
  trap cleanup EXIT
  trap 'exit 130' INT TERM HUP

  check
  if ((only)); then
    step "every check passed for $commit; nothing was tagged or upgraded"
    return 0
  fi
  upgrade
}

main "$@"
