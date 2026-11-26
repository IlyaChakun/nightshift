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
stamp=$(date -u +%Y%m%dT%H%M%SZ)
dest=.pre-tenant-state/$stamp
# The agent user owns this HOME and could have made .pre-tenant-state itself, or a link, to
# catch what is moved. Only a real directory of the user running this script is used; anything
# else is set aside unread.
# find tests the entry itself (no link is followed), the same way on Linux and macOS.
if { [ -e .pre-tenant-state ] || [ -L .pre-tenant-state ]; } &&
  [ -z "$(find .pre-tenant-state -maxdepth 0 -type d -user "$(id -u)")" ]; then
  mv -- .pre-tenant-state ".pre-tenant-state.untrusted-$stamp"
  echo "set aside .pre-tenant-state: not a directory of uid $(id -u)"
fi
moved=""
for name in .claude .claude.json .codex; do
  [ -e "$name" ] || [ -L "$name" ] || continue
  mkdir -p "$dest"
  chmod 700 .pre-tenant-state "$dest"
  mv -- "$name" "$dest/"
  moved="$moved $name"
done
echo "legacy home state moved:${moved:- nothing}"
