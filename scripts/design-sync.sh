#!/usr/bin/env bash
# Keeps docs/design.html in step with the design: docs/design.md and its layer files docs/design/*.md.
#
# The page carries a manifest that lists every top-level design section as
# "covered <hash>" (the page shows it) or "skipped" (the page does not). The
# check fails when a section is missing from the manifest, when the manifest
# names a section the design no longer has, or when a covered section changed
# since its hash was recorded. After updating the page, record the new hashes:
#
#   scripts/design-sync.sh --update
set -euo pipefail
cd "$(dirname "$0")/.."

design=docs/design.md
page=docs/design.html
[[ -f $design && -f $page ]] || exit 0

# design_files sets $files to the whole design: the overview, then each layer file. Each file
# is read on its own, so a layer's title and intro never join the last section of another file.
files=("$design")
for f in docs/design/*.md; do
  if [[ -f $f ]]; then files+=("$f"); fi
done

# section_hash N prints a short sha256 of section N, from its "## N." heading
# to the next top-level heading outside a code fence. Blank lines at the end of a section are
# left out, so a section hashes the same at the end of a file and before another section.
section_hash() {
  awk -v n="$1" '
    FNR == 1 { fence = 0; on = 0; blank = 0 }
    /^```/ { fence = !fence }
    !fence && /^# / { on = 0 }
    !fence && /^## [0-9]+\. / { split($2, a, "."); on = (a[1] == n); blank = 0 }
    on && !fence && /^[[:space:]]*$/ { blank++; next }
    on { while (blank > 0) { print ""; blank-- } print }' "${files[@]}" | sha256 | cut -c1-12
}

# sha256 works with GNU coreutils and with the macOS shasum.
sha256() {
  if command -v sha256sum >/dev/null; then sha256sum; else shasum -a 256; fi
}

# Plain strings rather than arrays, so the macOS bash 3.2 runs this too.
sections=$(awk 'FNR == 1 { f = 0 } /^```/ { f = !f } !f && /^## [0-9]+\. / { split($2, a, "."); print a[1] }' "${files[@]}")
dup=$(sort <<<"$sections" | uniq -d | tr '\n' ' ')
if [[ -n $dup ]]; then
  echo "design-sync: the design has a section number in two places: $dup(a section lives in one file)" >&2
  exit 1
fi
manifest=$(sed -n '/^<!-- design-sync$/,/^-->$/p' "$page" | grep -E '^§[0-9]+ ' || true)

if [[ -z $manifest ]]; then
  echo "design-sync: $page has no design-sync manifest" >&2
  exit 1
fi

# A merge can leave conflict markers or the same section twice in the manifest; both make
# the hashes below meaningless, so they fail in the check and in --update alike.
block=$(sed -n '/^<!-- design-sync$/,/^-->$/p' "$page" | sed '1d;$d')
stray=$(grep -nvE '^§[0-9]+ (covered [0-9a-f]+|skipped)( |$)' <<<"$block" || true)
if [[ -n $stray ]]; then
  echo "design-sync: the manifest of $page has lines that are not sections" \
    "(a merge conflict?); fix them by hand:" >&2
  echo "$stray" >&2
  exit 1
fi
twice=$(cut -d' ' -f1 <<<"$manifest" | sort | uniq -d | tr '\n' ' ')
if [[ -n $twice ]]; then
  echo "design-sync: the manifest of $page lists a section twice: $twice" >&2
  exit 1
fi

if [[ ${1:-} == --update ]]; then
  tmp=$(mktemp)
  while IFS= read -r line; do
    if [[ $line =~ ^§([0-9]+)\ covered\ [0-9a-f]+(.*)$ ]]; then
      echo "§${BASH_REMATCH[1]} covered $(section_hash "${BASH_REMATCH[1]}")${BASH_REMATCH[2]}"
    else
      echo "$line"
    fi
  done <"$page" >"$tmp"
  mv "$tmp" "$page"
  echo "design-sync: hashes updated; check that the page reflects the changed sections"
  exit 0
fi

fail=0
listed=$(cut -d' ' -f1 <<<"$manifest" | tr -d '§')
while IFS= read -r line; do
  read -r tag state hash _ <<<"$line"
  n=${tag#§}
  if ! grep -qx "$n" <<<"$sections"; then
    echo "design-sync: $page lists §$n, which the design does not have" >&2
    fail=1
  elif [[ $state == covered && $hash != "$(section_hash "$n")" ]]; then
    echo "design-sync: §$n of the design changed since $page was reviewed;" \
      "update the page, then run scripts/design-sync.sh --update" >&2
    fail=1
  elif [[ $state != covered && $state != skipped ]]; then
    echo "design-sync: §$n has state '$state'; use covered or skipped" >&2
    fail=1
  fi
done <<<"$manifest"
for n in $sections; do
  if ! grep -qx "$n" <<<"$listed"; then
    echo "design-sync: §$n of the design is not in the manifest of $page;" \
      "add it as covered or skipped" >&2
    fail=1
  fi
done
exit "$fail"
