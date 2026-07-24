#!/usr/bin/env bash
# Fail if production Rust source exceeds 500 non-blank, non-comment lines.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MAX=500
fail=0
while IFS= read -r f; do
  [[ "$f" == *"/tests/"* ]] && continue
  n=$(grep -v '^\s*$' "$f" | grep -v '^\s*//' | grep -v '^\s*#' | wc -l | tr -d ' ')
  if [[ "$n" -gt "$MAX" ]]; then
    echo "LOC $n > $MAX: $f"
    fail=1
  fi
done < <(find "$ROOT/crates" -name '*.rs' -print)
exit "$fail"
