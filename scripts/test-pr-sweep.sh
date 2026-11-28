#!/usr/bin/env bash
# Runs scripts/pr-sweep.sh against a stub `gh` and checks which PRs get a comment.
set -euo pipefail
cd "$(dirname "$0")/.."

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
now=$(date +%s)
iso() { jq -nr --argjson stamp "$1" '$stamp | todateiso8601'; }
old=$(iso $((now - 5 * 3600)))
fresh=$(iso $((now - 600)))
recent=$(iso $((now - 3600)))
long=$(iso $((now - 30 * 3600)))

check='[{"name":"ci-ok"}]'
cat >"$tmp/list.json" <<J
[
 {"number":1,"title":"conflicting","isDraft":false,"createdAt":"$fresh","mergeable":"CONFLICTING","statusCheckRollup":$check,"commits":[{"committedDate":"$fresh"}]},
 {"number":2,"title":"fine","isDraft":false,"createdAt":"$fresh","mergeable":"MERGEABLE","statusCheckRollup":$check,"commits":[{"committedDate":"$fresh"}]},
 {"number":3,"title":"no checks","isDraft":false,"createdAt":"$recent","mergeable":"MERGEABLE","statusCheckRollup":[],"commits":[{"committedDate":"$recent"}]},
 {"number":4,"title":"idle draft","isDraft":true,"createdAt":"$old","mergeable":"MERGEABLE","statusCheckRollup":$check,"commits":[{"committedDate":"$old"}]},
 {"number":5,"title":"idle but told today","isDraft":false,"createdAt":"$old","mergeable":"MERGEABLE","statusCheckRollup":$check,"commits":[{"committedDate":"$old"}]},
 {"number":6,"title":"new, checks not started yet","isDraft":false,"createdAt":"$fresh","mergeable":"MERGEABLE","statusCheckRollup":[],"commits":[{"committedDate":"$fresh"}]},
 {"number":7,"title":"idle, told long ago","isDraft":false,"createdAt":"$old","mergeable":"MERGEABLE","statusCheckRollup":$check,"commits":[{"committedDate":"$old"}]},
 {"number":8,"title":"idle, marker typed by a person","isDraft":false,"createdAt":"$old","mergeable":"MERGEABLE","statusCheckRollup":$check,"commits":[{"committedDate":"$old"}]},
 {"number":9,"title":"checks unreadable","isDraft":false,"createdAt":"$old","mergeable":"MERGEABLE","statusCheckRollup":null,"commits":[{"committedDate":"$fresh"}]},
 {"number":10,"title":"idle and conflicting but parked","isDraft":true,"createdAt":"$old","mergeable":"CONFLICTING","statusCheckRollup":[],"labels":[{"name":"research"},{"name":"parked"}],"commits":[{"committedDate":"$old"}]},
 {"number":11,"title":"idle, other label","isDraft":false,"createdAt":"$old","mergeable":"MERGEABLE","statusCheckRollup":$check,"labels":[{"name":"parked-later"}],"commits":[{"committedDate":"$old"}]}
]
J
mkdir "$tmp/bin"
cat >"$tmp/bin/gh" <<G
#!/usr/bin/env bash
case "\$1 \$2" in
  "pr list")
    # GitHub refuses commits in a list of many PRs (the node limit); so does this stub.
    if [[ "\$*" == *commits* ]]; then echo "GraphQL: exceeds the maximum limit" >&2; exit 1; fi
    cat "$tmp/list.json" ;;
  "pr view")
    if [[ \$5 == commits ]]; then
      jq -r --argjson n "\$3" '.[] | select(.number == \$n) | {commits} | '"\${@: -1}" "$tmp/list.json"
      exit
    fi
    case \$3 in
      5) c='{"comments":[{"author":{"login":"github-actions"},"body":"<!-- pr-sweep -->\nx","createdAt":"$recent"}]}' ;;
      7) c='{"comments":[{"author":{"login":"github-actions"},"body":"<!-- pr-sweep -->\nx","createdAt":"$long"}]}' ;;
      8) c='{"comments":[{"author":{"login":"someone"},"body":"<!-- pr-sweep -->","createdAt":"$recent"}]}' ;;
      *) c='{"comments":[]}' ;;
    esac
    jq -r "\${@: -1}" <<<"\$c" ;;
  "pr comment") echo "\$3" >>"$tmp/commented" ;;
esac
G
chmod +x "$tmp/bin/gh"

PATH="$tmp/bin:$PATH" GITHUB_STEP_SUMMARY="$tmp/summary" ./scripts/pr-sweep.sh >/dev/null
got=$(sort -n "$tmp/commented" | tr '\n' ' ')
want="1 3 4 7 8 11 "
if [[ $got != "$want" ]]; then
  echo "test-pr-sweep: commented on '$got', want '$want'" >&2
  exit 1
fi
for n in '#1' '#3' '#4 (draft)' '#5' '#7' '#8' '#11'; do
  grep -q -- "- $n " "$tmp/summary" || { echo "test-pr-sweep: $n missing from the summary" >&2; exit 1; }
done
if grep -q -- '- #2 \|- #6 \|- #9 \|- #10 ' "$tmp/summary"; then
  echo "test-pr-sweep: a healthy or parked PR was listed" >&2
  exit 1
fi
echo "test-pr-sweep: ok"
