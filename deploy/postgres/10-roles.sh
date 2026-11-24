#!/bin/sh
# Runs once, on an empty data directory. Creates the schema owner and the login of the
# nightshift role; migrations create the rest (design section 3).
set -eu
owner_pw=$(cat /run/secrets/pg_owner_password)
service_pw=$(cat /run/secrets/pg_nightshift_password)
psql -v ON_ERROR_STOP=1 -v owner_pw="$owner_pw" -v service_pw="$service_pw" --username postgres --dbname book <<'SQL'
CREATE ROLE book_owner LOGIN CREATEROLE PASSWORD :'owner_pw';
CREATE ROLE nightshift LOGIN PASSWORD :'service_pw';
ALTER DATABASE book OWNER TO book_owner;
ALTER SCHEMA public OWNER TO book_owner;
REVOKE ALL ON DATABASE book FROM PUBLIC;
GRANT CONNECT ON DATABASE book TO nightshift;
SQL
