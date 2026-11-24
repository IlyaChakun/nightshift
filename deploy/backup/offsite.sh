#!/bin/sh
# Copies the dumps to the restic repository, encrypted, and thins old snapshots.
set -eu
. /etc/backup/env.sh
restic cat config >/dev/null 2>&1 || restic init
restic backup --host nightshift --tag book /dumps
restic forget --prune --keep-daily 14 --keep-weekly 8 --keep-monthly 12
