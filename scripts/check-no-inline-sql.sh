#!/usr/bin/env bash
# No inline SQL in production Rust: every query lives in sql/*.sql, bound via
# include_str! and surfaced as a crate::sql:: const. Tests (src/tests, /tests/)
# are exempt. Error text that names a SQL word carries a `// sql-gate:allow` marker.
# ponytail: grep heuristic — extend the marker or keyword list if it misfires.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/crates/core/src"
KW='SELECT|INSERT|UPDATE|DELETE|MERGE|CREATE|DROP|ALTER|EXEC|OBJECT_ID|DB_ID'

raw=$(grep -rnE "\"[^\"]*\b(${KW})\b" "$SRC" --include='*.rs' \
  | grep -v '/tests/' \
  | grep -v 'include_str!' \
  | grep -vE ':[0-9]+:[[:space:]]*//' || true)

# A `// sql-gate:allow` marker on the hit line or an adjacent line (rustfmt may
# push a long line's trailing comment onto its own line) clears the hit.
hits=""
while IFS= read -r line; do
  [ -z "$line" ] && continue
  f=${line%%:*}; rest=${line#*:}; n=${rest%%:*}
  if sed -n "$((n > 1 ? n - 1 : 1)),$((n + 1))p" "$f" | grep -q 'sql-gate:allow'; then
    continue
  fi
  hits+="$line"$'\n'
done <<< "$raw"
hits=${hits%$'\n'}

if [ -n "$hits" ]; then
  echo "check-no-inline-sql: inline SQL found — move to sql/*.sql + crate::sql::" >&2
  echo "(or, for non-query error text, append '// sql-gate:allow'):" >&2
  echo "$hits" >&2
  exit 1
fi
echo "check-no-inline-sql: PASS"
