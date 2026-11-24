#!/bin/sh
# Restores the newest dump into a scratch database and counts its tasks. A failure here
# means the backups cannot be trusted: fix it before anything else (docs/runbook.md).
set -eu
. /etc/backup/env.sh
latest=$(ls -1t /dumps/book-*.dump | head -n 1)
dropdb --if-exists restore_test
createdb restore_test
trap 'dropdb --if-exists restore_test' EXIT
pg_restore --exit-on-error --dbname=restore_test "$latest"
tasks=$(psql -tA -d restore_test -c 'SELECT count(*) FROM book.tasks')
events=$(psql -tA -d restore_test -c 'SELECT count(*) FROM book.events')
date -u +%Y-%m-%dT%H:%M:%SZ >/dumps/restore-test.ok
echo "restore test: ok, $latest, $tasks tasks, $events events"
