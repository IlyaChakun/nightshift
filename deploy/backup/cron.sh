#!/bin/sh
# busybox crond skips crontabs not owned by root, and bind mounts keep the host owner,
# so copy the crontab first. Usage: cron.sh <directory under /etc/backup>.
set -eu
mkdir -p /tmp/crontabs
cp "/etc/backup/$1/root" /tmp/crontabs/root
exec crond -f -c /tmp/crontabs
