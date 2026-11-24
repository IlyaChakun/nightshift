#!/bin/sh
# Dumps the whole book database; keeps the newest $KEEP_DUMPS dumps.
set -eu
. /etc/backup/env.sh
out=/dumps/book-$(date -u +%Y%m%dT%H%M%SZ).dump
pg_dump --format=custom --file="$out.part"
mv "$out.part" "$out"
ls -1t /dumps/book-*.dump | tail -n +$((KEEP_DUMPS + 1)) | xargs -r rm --
echo "backup: wrote $out"
