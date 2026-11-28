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

# The drill's preview network takes its own subnet, outside Docker's default pools, so it never
# overlaps a running instance's (an exported NIGHTSHIFT_PREVIEW_SUBNET belongs to that instance).
preview_subnet=${NIGHTSHIFT_DRILL_PREVIEW_SUBNET:-10.231.252.0/24}
compose() { NIGHTSHIFT_PREVIEW_SUBNET=$preview_subnet docker compose -p "$project" -f "$repo/deploy/docker-compose.yml" "$@"; }

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
agent() { agent_as 20001:20001 "$1"; }
agent_as() {
  compose run --rm --no-deps -T --user "$1" --entrypoint bash worker -c "$2" </dev/null
}

# blocked <name> <command>: ok when the command, run as the agent, fails. The wrapper prints
# an end marker after it and a leak marker when the command worked, both with a random suffix, so a container that did not start cannot pass.
blocked() {
  local name=$1 out n=$RANDOM$RANDOM
  total=$((total + 1))
  out=$(agent "if ( $2 ) >/dev/null 2>&1; then echo DRILL_LEAK_$n; fi; echo DRILL_END_$n" 2>&1)
  if [[ $out == *DRILL_END_$n* && $out != *DRILL_LEAK_$n* ]]; then
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
# init-secrets.sh keeps existing files, so it is safe to call whenever the last one it writes is missing.
if [[ ! -f $repo/deploy/secrets/restic_repository ]]; then
  if [[ $(uname -s) != Darwin && $(id -u) != 0 ]]; then
    echo "drill: deploy/secrets is incomplete; run once: sudo deploy/init-secrets.sh" >&2
    exit 2
  fi
  "$repo/deploy/init-secrets.sh" || {
    echo "drill: could not create deploy/secrets (see docs/runbook.md)" >&2
    exit 2
  }
fi
trap cleanup EXIT
compose down -v --remove-orphans >/dev/null 2>&1

echo "drill: phase 1, the book"
check "postgres starts" compose up -d --wait postgres
# The healthcheck can pass on the temporary server of the image's init; wait for the real one
# and for the roles that the init script creates.
roles_ready() {
  local i
  for i in $(seq 1 60); do
    [[ $(compose exec -T postgres psql -U postgres -h 127.0.0.1 -d book -tA \
      -c "SELECT count(*) FROM pg_roles WHERE rolname IN ('book_owner', 'nightshift', 'book_backup')" 2>/dev/null) == 3 ]] && return 0
    sleep 2
  done
  return 1
}
check "the book database and its roles exist" roles_ready
check "migrations apply" compose run --rm -T migrate
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
  compose rm -sf postgres >/dev/null && compose up -d --wait postgres >/dev/null && roles_ready && marker_present
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
check "the worker's token is mounted (read as root, the control for the next check)" \
  agent_as 0 'test -s /run/worker-secrets/worker_token'
blocked "the agent finds no GitHub, chat or tracker token in its environment" \
  'env | grep -iE "(^|_)gh_|github|slack|chat|linear|jira"'
blocked "the agent cannot read the worker's secrets" \
  'cat /run/worker-secrets/worker_token'
blocked "the agent sees no secret of the core in /run/secrets" \
  'for f in /run/secrets/*; do [ -e "$f" ] && exit 0; done; exit 1'
check "an allowlisted host answers through the proxy" \
  agent 'curl -sS --max-time 20 -o /dev/null -x "$HTTPS_PROXY" https://api.anthropic.com/'
# The proxy must answer the CONNECT with a refusal: 200 is a leak and 000 means no answer at all.
blocked "a host outside the allowlist is refused by the proxy" \
  'c=$(curl -s -o /dev/null -w "%{http_connect}" --max-time 20 -x "$HTTPS_PROXY" https://example.com/); [ "$c" = 200 ] || [ "$c" = 000 ]'
blocked "a private address is refused by the proxy" \
  'c=$(curl -s -o /dev/null -w "%{http_connect}" --max-time 20 -x "$HTTPS_PROXY" https://10.0.0.1/); [ "$c" = 200 ] || [ "$c" = 000 ]'
blocked "the worker has no direct way out" \
  'env -u HTTPS_PROXY -u https_proxy curl -sS --max-time 10 -o /dev/null https://api.anthropic.com/'
check "the shell can open a TCP connection (the control for the next check)" \
  agent 'timeout 5 bash -c "</dev/tcp/egress/4750"'
blocked "the worker cannot reach Postgres" \
  'timeout 5 bash -c "</dev/tcp/postgres/5432"'

echo "drill: $total checks, $failed failed"
((failed == 0))
