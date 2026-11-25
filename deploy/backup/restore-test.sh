#!/bin/sh
# Restores the newest dump into a scratch database and counts its tasks. A failure here
# means the backups cannot be trusted: fix it before anything else (docs/runbook.md).
set -eu
. /etc/backup/env.sh
latest=$(ls -1t /dumps/book-*.dump | head -n 1)
dropdb --if-exists restore_test
createdb restore_test
# The scratch database comes from template1, which already holds the extensions (the dump's own
# EXTENSION entries would fail: book_backup cannot create them).
list=$(mktemp)
trap 'rm -f "$list"; dropdb --if-exists restore_test' EXIT
if [ "$(psql -tA -d restore_test -c "SELECT count(*) FROM pg_extension WHERE extname = 'vector'")" != 1 ]; then
	echo "restore test: template1 lacks the vector extension; run once as the superuser: psql -U postgres -d template1 -c 'CREATE EXTENSION vector' (docs/runbook.md)" >&2
	exit 1
fi
pg_restore --list "$latest" | grep -v ' EXTENSION ' >"$list"
pg_restore --exit-on-error --no-owner --no-privileges --use-list="$list" --dbname=restore_test "$latest"
tasks=$(psql -tA -d restore_test -c 'SELECT count(*) FROM book.tasks')
events=$(psql -tA -d restore_test -c 'SELECT count(*) FROM book.events')
date -u +%Y-%m-%dT%H:%M:%SZ >/dumps/restore-test.ok
echo "restore test: ok, $latest, $tasks tasks, $events events"
