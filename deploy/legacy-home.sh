#!/bin/sh
# Moves the CLI state that was kept straight in the shared agent HOME before each tenant got its
# own (#310) into <home>/.pre-tenant-state/<time>, a directory only root can read, so that no
# tenant's job reads those sessions and logins. It never deletes anything; a person may look at
# the copy and remove it. deploy/upgrade.sh runs it as root in a one-off worker container.
#
#   sh deploy/legacy-home.sh [home]     default home: /home/agent
set -eu
home=${1:-/home/agent}
cd "$home"
dest=.pre-tenant-state/$(date -u +%Y%m%dT%H%M%SZ)
moved=""
for name in .claude .claude.json .codex; do
  [ -e "$name" ] || [ -L "$name" ] || continue
  mkdir -p "$dest"
  chmod 700 .pre-tenant-state "$dest"
  mv -- "$name" "$dest/"
  moved="$moved $name"
done
echo "legacy home state moved:${moved:- nothing}"
