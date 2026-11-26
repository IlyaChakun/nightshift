#!/usr/bin/env bash
# Runs a tagged release of the factory as its own compose project, beside the development stack
# in this checkout (docs/runbook.md, "Stable instance").
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
state=${NIGHTSHIFT_STABLE_HOME:-$HOME/.nightshift/stable}
secrets=${NIGHTSHIFT_STABLE_SECRETS:-$repo/deploy/secrets}
# Compose reads a relative config path as a volume name, and a relative link would dangle.
for dir in "$state" "$secrets"; do
  [[ $dir == /* ]] || {
    echo "stable: $dir must be an absolute path" >&2
    exit 1
  }
done
project=nightshift-stable

die() {
  echo "stable: $*" >&2
  exit 1
}

usage() {
  cat >&2 <<'USAGE'
usage: scripts/stable.sh up <tag>              build the tag, start it and run doctor
       scripts/stable.sh compose <args>...     docker compose on the running release
       scripts/stable.sh current               print the running tag
USAGE
  exit 2
}

# compose <tag> <args>...: docker compose on one release, with this instance's config.
compose() {
  local dir=$state/releases/$1
  shift
  NIGHTSHIFT_CONFIG=$state/config NIGHTSHIFT_OFFSITE=$state/offsite \
    docker compose -p "$project" --env-file "$state/env" -f "$dir/deploy/docker-compose.yml" \
    --profile obs "$@"
}

# fill_secrets <deploy dir> <release>: a release may add a generated secret. init-secrets.sh creates only
# the missing files and keeps the others; on Linux it needs root to set their owners.
fill_secrets() {
  local f missing=0
  [[ -f $1/docker-compose.yml ]] || return 0
  for f in $(sed -nE 's#.*file: \./secrets/([^ }]+).*#\1#p' "$1/docker-compose.yml"); do
    [[ -e $1/secrets/$f ]] || missing=1
  done
  ((missing)) || return 0
  "$1/init-secrets.sh" >/dev/null || die "secrets are missing; run as root the deploy/init-secrets.sh of $2"
}

current() {
  [[ -f $state/current ]] || die "nothing is running yet: scripts/stable.sh up <tag>"
  cat "$state/current"
}

# export_tag <tag>: the sources of the tag, written once. Tags do not move.
export_tag() {
  local tag=$1 dir=$state/releases/$1 commit
  commit=$(git -C "$repo" rev-parse -q --verify "refs/tags/$tag^{commit}") || die "no tag $tag"
  if [[ -d $dir ]]; then
    [[ $(cat "$dir/.commit") == "$commit" ]] || die "tag $tag moved; make a new tag instead"
  else
    rm -rf "$dir.partial"
    mkdir -p "$dir.partial"
    git -C "$repo" archive "$commit" | tar -x -C "$dir.partial"
    echo "$commit" >"$dir.partial/.commit"
    mv "$dir.partial" "$dir"
  fi
  ln -sfn "$secrets" "$dir/deploy/secrets"
}

up() {
  local tag=$1 previous="" i
  [[ $tag =~ ^[A-Za-z0-9._-]+$ ]] || die "a tag may hold letters, digits, dots, dashes and underscores"
  [[ -f $state/config/instance.yaml ]] || die "no config in $state/config"
  [[ -f $state/env ]] || die "no $state/env (CLAUDE_CODE_VERSION, CODEX_VERSION)"
  [[ -d $secrets ]] || die "no secrets directory $secrets"
  if [[ -f $state/current ]]; then previous=$(current); fi
  mkdir -p "$state/offsite"
  export_tag "$tag"
  fill_secrets "$state/releases/$tag/deploy" "$tag"

  # The running containers stay as they are; images that did build are replaced, so the
  # current tag must be built again before anything recreates a container.
  compose "$tag" build ||
    die "build of $tag failed${previous:+; restore the images with: scripts/stable.sh up $previous}"
  if [[ -n $previous && $previous != "$tag" ]]; then echo "$previous" >"$state/previous"; fi
  echo "$tag" >"$state/current"
  if [[ -f $state/previous ]]; then
    echo "stable: to go back: scripts/stable.sh up $(cat "$state/previous")"
  fi
  compose "$tag" up -d --remove-orphans
  for ((i = 0; i < 30; i++)); do
    compose "$tag" exec -T nightshift nightshift status --health >/dev/null 2>&1 && break
    sleep 2
  done
  compose "$tag" exec -T nightshift nightshift doctor
  compose "$tag" exec -T worker nightshift doctor --worker
  echo "stable: $tag is running"
}

case ${1:-} in
up)
  [[ $# -eq 2 ]] || usage
  up "$2"
  ;;
compose)
  shift
  tag=$(current)
  compose "$tag" "$@"
  ;;
current) current ;;
*) usage ;;
esac
