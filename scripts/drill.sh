#!/usr/bin/env bash
# Runs the live "Done" checks of phases 1 and 2 against a throwaway compose project and prints
# one line per check (docs/runbook.md, "Drill"). Needs Docker; it never touches the development
# stack: its own project name, its own volumes, and no `nightshift` service, so it never
# connects to chat or GitHub.
#
#   scripts/drill.sh          run every check, then remove the drill project
#   scripts/drill.sh --keep   leave the drill project running for a look
#
# Checks: the book restores from a dump by the runbook steps and survives a restart of Postgres
# (phase 1); the agent user in the worker has no token and no way to a host outside the allowlist,
# the local network or Postgres (phase 2).
set -uo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
project=nightshift-drill
keep=0
[[ ${1:-} == --keep ]] && keep=1

compose() { docker compose -p "$project" -f "$repo/deploy/docker-compose.yml" "$@"; }

total=0
failed=0
# check <name> <command>...: runs the command; ok when it exits 0.
check() {
  local name=$1 out
  shift
  total=$((total + 1))
  if out=$("$@" 2>&1); then
    echo "ok   $name"
  else
    failed=$((failed + 1))
    echo "FAIL $name"
    printf '     %s\n' "${out:0:600}" | head -n 8
  fi
}

# agent <script>: runs bash as the agent user in a fresh worker container, with the worker's
# environment and networks but without the runner.
agent() {
  compose run --rm --no-deps -T --user 20001:20001 --entrypoint bash worker -c "$1" </dev/null
}

# blocked <name> <command>: ok when the command, run as the agent, fails. The wrapper prints
# DONE after it and LEAK when the command worked, so a container that did not start cannot pass.
blocked() {
  local name=$1 out
  total=$((total + 1))
  out=$(agent "if ( $2 ) >/dev/null 2>&1; then echo LEAK; fi; echo DONE" 2>&1)
  if [[ $out == *DONE* && $out != *LEAK* ]]; then
    echo "ok   $name"
  else
    failed=$((failed + 1))
    echo "FAIL $name"
    printf '     %s\n' "${out:0:400}" | head -n 6
  fi
}

cleanup() {
  if ((keep)); then
    echo "drill: left running, remove with: docker compose -p $project -f deploy/docker-compose.yml down -v"
  else
    compose down -v --remove-orphans >/dev/null 2>&1
  fi
}

command -v docker >/dev/null || {
  echo "drill: docker is not installed" >&2
  exit 2
}
docker info >/dev/null 2>&1 || {
  echo "drill: the Docker daemon does not answer" >&2
  exit 2
}
# The secrets are files in deploy/secrets; create the ones that are missing (existing ones are kept).
[[ -f $repo/deploy/secrets/pg_admin ]] || "$repo/deploy/init-secrets.sh" ||
  {
    echo "drill: could not create deploy/secrets (see docs/runbook.md)" >&2
    exit 2
  }
trap cleanup EXIT
compose down -v --remove-orphans >/dev/null 2>&1

echo "drill: phase 1, the book"
check "postgres starts" compose up -d --wait postgres
check "migrations apply" compose run --rm migrate
marker="drill-$(date -u +%s)"
check "write a marker row" compose exec -T postgres psql -U postgres -d book -v ON_ERROR_STOP=1 \
  -c "CREATE TABLE IF NOT EXISTS book.drill_marker (v text)" -c "INSERT INTO book.drill_marker VALUES ('$marker')"
marker_present() {
  [[ $(compose exec -T postgres psql -U postgres -d book -tA -c "SELECT count(*) FROM book.drill_marker WHERE v = '$marker'") == 1 ]]
}
check "the marker is readable" marker_present
check "backup starts" compose up -d --wait backup
check "a dump is written" compose exec -T backup /etc/backup/dump.sh
check "the restore test passes" compose exec -T backup /etc/backup/restore-test.sh
restart_survives() {
  compose restart postgres >/dev/null && compose up -d --wait postgres >/dev/null && marker_present
}
check "state survives a restart of postgres" restart_survives
# The runbook restore: drop the book, restore it from the dump as the superuser (docs/runbook.md).
restore() {
  local dump
  dump=$(compose exec -T backup sh -c 'ls -1t /dumps/book-*.dump | head -n 1' | tr -d '\r') || return 1
  [[ -n $dump ]] || return 1
  compose exec -T backup cat "$dump" | compose exec -T postgres sh -c 'cat >/tmp/book.dump' || return 1
  compose exec -T postgres sh -c 'dropdb -U postgres --force book &&
    pg_restore -U postgres --create --exit-on-error -d postgres /tmp/book.dump && rm /tmp/book.dump' || return 1
  marker_present
}
check "the book restores from a dump by the runbook" restore

echo "drill: phase 2, the agent"
check "the egress proxy starts" compose up -d --wait egress
check "the agent shell runs as uid 20001" agent '[[ $(id -u) == 20001 ]]'
blocked "the agent finds no GitHub, chat or tracker token in its environment" \
  'env | grep -iE "(github|slack|chat|linear|jira)[a-z_]*="'
blocked "the agent cannot read the worker's secrets" \
  'cat /run/worker-secrets/worker_token'
blocked "the agent has no secret of the core in /run/secrets" \
  'ls /run/secrets/github_token /run/secrets/chat_bot_token /run/secrets/pg_nightshift'
check "an allowlisted host answers through the proxy" \
  agent 'curl -sS --max-time 20 -o /dev/null -x "$HTTPS_PROXY" https://api.anthropic.com/'
blocked "a host outside the allowlist is blocked by the proxy" \
  'curl -sS --max-time 20 -o /dev/null -x "$HTTPS_PROXY" https://example.com/'
blocked "a private address is blocked by the proxy" \
  'curl -sS --max-time 10 -o /dev/null -x "$HTTPS_PROXY" https://10.0.0.1/'
blocked "the worker has no direct way out" \
  'env -u HTTPS_PROXY -u https_proxy curl -sS --max-time 10 -o /dev/null https://api.anthropic.com/'
blocked "the worker cannot reach Postgres" \
  'timeout 5 bash -c "</dev/tcp/postgres/5432"'

echo "drill: $total checks, $failed failed"
((failed == 0))
