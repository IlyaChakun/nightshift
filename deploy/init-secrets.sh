#!/bin/sh
# Creates deploy/secrets/ with random Postgres passwords and the restic password, each
# file 0400 and owned by the uid of the container that reads it. Run once, as root:
#   sudo deploy/init-secrets.sh
# Chat tokens are pasted by hand afterwards (docs/runbook.md). Existing files are kept.
set -eu
dir=$(dirname "$0")/secrets
mkdir -p "$dir"
chmod 0711 "$dir"

pg_uid=999       # postgres in the pgvector (Debian) image
ns_uid=10001     # nightshift and migrate

rand() { od -An -N24 -tx1 /dev/urandom | tr -d ' \n'; }
put() {          # put <name> <uid> <value>
	[ -e "$dir/$1" ] && return 0
	umask 077
	printf '%s' "$3" >"$dir/$1"
	chown "$2" "$dir/$1"
	chmod 0400 "$dir/$1"
}

keep() { cat "$dir/$1" 2>/dev/null || rand; }   # reuse a password that already exists
owner_pw=$(keep pg_owner_password)
service_pw=$(keep pg_nightshift_password)
put pg_admin "$pg_uid" "$(rand)"
put pg_owner_password "$pg_uid" "$owner_pw"
put pg_nightshift_password "$pg_uid" "$service_pw"
put pg_backup_password "$pg_uid" "$(rand)"   # backup runs as root and reads it too
put pg_owner "$ns_uid" "postgres://book_owner:$owner_pw@postgres:5432/book?sslmode=disable"
put pg_nightshift "$ns_uid" "postgres://nightshift:$service_pw@postgres:5432/book?sslmode=disable"
if [ ! -e "$dir/worker_token" ]; then
	token=$(rand)
	put worker_token 0 "$token"
	put worker_token.sha256 "$ns_uid" "$(printf '%s' "$token" | sha256sum | cut -d' ' -f1)"
fi
if [ ! -d "$dir/harness" ]; then   # one file per credential, for example CLAUDE_CODE_OAUTH_TOKEN
	mkdir -m 0700 "$dir/harness"
fi
put restic_password 0 "$(rand)"
put restic_repository 0 "/offsite/nightshift"
for f in chat_bot_token chat_app_token; do
	[ -e "$dir/$f" ] || echo "missing $dir/$f: paste the token, then chown $ns_uid and chmod 0400"
done
echo "Store restic_password in your password manager: without it the off-site copy is lost."
