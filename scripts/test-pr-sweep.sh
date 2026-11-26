#!/usr/bin/env bash
# Runs scripts/pr-sweep.sh against a stub `gh` and checks which PRs get a comment.
set -euo pipefail
cd "$(dirname "$0")/.."

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
now=$(date +%s)
iso() { date -u -d "@$1" +%Y-%m-%dT%H:%M:%SZ; }
old=$(iso $((now - 5 * 3600)))
fresh=$(iso $((now - 600)))
recent=$(iso $((now - 3600)))

check='[{"name":"ci-ok"}]'
cat >"$tmp/list.json" <<J
[
 {"number":1,"title":"conflicting","isDraft":false,"createdAt":"$fresh","mergeable":"CONFLICTING","statusCheckRollup":$check,"commits":[{"committedDate":"$fresh"}]},
 {"number":2,"title":"fine","isDraft":false,"createdAt":"$fresh","mergeable":"MERGEABLE","statusCheckRollup":$check,"commits":[{"committedDate":"$fresh"}]},
 {"number":3,"title":"no checks","isDraft":false,"createdAt":"$recent","mergeable":"MERGEABLE","statusCheckRollup":[],"commits":[{"committedDate":"$recent"}]},
 {"number":4,"title":"idle draft","isDraft":true,"createdAt":"$old","mergeable":"MERGEABLE","statusCheckRollup":$check,"commits":[{"committedDate":"$old"}]},
 {"number":5,"title":"idle but told today","isDraft":false,"createdAt":"$old","mergeable":"MERGEABLE","statusCheckRollup":$check,"commits":[{"committedDate":"$old"}]},
 {"number":6,"title":"new, checks not started yet","isDraft":false,"createdAt":"$fresh","mergeable":"MERGEABLE","statusCheckRollup":[],"commits":[{"committedDate":"$fresh"}]}
]
J
mkdir "$tmp/bin"
cat >"$tmp/bin/gh" <<G
#!/usr/bin/env bash
case "\$1 \$2" in
  "pr list") cat "$tmp/list.json" ;;
  "pr view")
    if [[ \$3 == 5 ]]; then echo "$recent"; fi ;;
  "pr comment") echo "\$3" >>"$tmp/commented" ;;
esac
G
chmod +x "$tmp/bin/gh"

PATH="$tmp/bin:$PATH" GITHUB_STEP_SUMMARY="$tmp/summary" ./scripts/pr-sweep.sh >/dev/null
got=$(sort -n "$tmp/commented" | tr '\n' ' ')
want="1 3 4 "
if [[ $got != "$want" ]]; then
  echo "test-pr-sweep: commented on '$got', want '$want'" >&2
  exit 1
fi
for n in '#1' '#3' '#4 (draft)' '#5'; do
  grep -q -- "- $n " "$tmp/summary" || { echo "test-pr-sweep: $n missing from the summary" >&2; exit 1; }
done
if grep -q -- '- #2 \|- #6 ' "$tmp/summary"; then
  echo "test-pr-sweep: a healthy PR was listed" >&2
  exit 1
fi
echo "test-pr-sweep: ok"
