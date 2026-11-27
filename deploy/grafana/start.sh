#!/bin/sh
# Starts the lgtm container of the obs profile. Grafana takes its admin password from the
# environment only when it creates its database, so on an existing data volume this sets it
# from the secret first. Without that an old volume would keep its first password.
set -eu
db=/data/grafana/data/grafana.db
if [ -f "$db" ]; then
	cd /otel-lgtm/grafana
	GF_PATHS_DATA=/data/grafana/data ./bin/grafana cli --homepath /otel-lgtm/grafana \
		admin reset-admin-password --password-from-stdin </run/secrets/grafana_admin_password >/dev/null ||
		{ echo "lgtm: cannot set the Grafana admin password" >&2; exit 1; }
	cd /otel-lgtm
fi
exec /otel-lgtm/run-all.sh
