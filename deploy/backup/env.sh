# Shared settings; crond does not pass the container environment to jobs.
export PGHOST=postgres PGUSER=postgres PGDATABASE=book
export PGPASSFILE=/tmp/pgpass
[ -r /run/secrets/pg_admin ] && printf 'postgres:5432:*:postgres:%s\n' "$(cat /run/secrets/pg_admin)" >"$PGPASSFILE" && chmod 0600 "$PGPASSFILE"
export RESTIC_REPOSITORY_FILE=/run/secrets/restic_repository RESTIC_PASSWORD_FILE=/run/secrets/restic_password
KEEP_DUMPS=${KEEP_DUMPS:-48}
