# Shared settings; crond does not pass the container environment to jobs.
# The backup role reads everything and owns only its scratch database; never the superuser.
export PGHOST=postgres PGUSER=book_backup PGDATABASE=book
export PGPASSFILE=/tmp/pgpass
[ -r /run/secrets/pg_backup_password ] && (umask 077 && printf 'postgres:5432:*:book_backup:%s\n' "$(cat /run/secrets/pg_backup_password)" >"$PGPASSFILE")
export RESTIC_REPOSITORY_FILE=/run/secrets/restic_repository RESTIC_PASSWORD_FILE=/run/secrets/restic_password
KEEP_DUMPS=${KEEP_DUMPS:-48}
