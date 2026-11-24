#!/bin/sh
# Runs once, on an empty data directory. Creates the schema owner, the login of the
# nightshift role and the read-only backup role; migrations create the rest (design section 3).
set -eu
owner_pw=$(cat /run/secrets/pg_owner_password)
service_pw=$(cat /run/secrets/pg_nightshift_password)
backup_pw=$(cat /run/secrets/pg_backup_password)
psql -v ON_ERROR_STOP=1 -v owner_pw="$owner_pw" -v service_pw="$service_pw" -v backup_pw="$backup_pw" --username postgres --dbname book <<'SQL'
CREATE ROLE book_owner LOGIN CREATEROLE PASSWORD :'owner_pw';
CREATE ROLE nightshift LOGIN PASSWORD :'service_pw';
-- Dumps read everything; the weekly restore test creates and drops its own scratch database.
CREATE ROLE book_backup LOGIN CREATEDB PASSWORD :'backup_pw' IN ROLE pg_read_all_data;
ALTER DATABASE book OWNER TO book_owner;
ALTER SCHEMA public OWNER TO book_owner;
REVOKE ALL ON DATABASE book FROM PUBLIC;
GRANT CONNECT ON DATABASE book TO nightshift, book_backup;
SQL
