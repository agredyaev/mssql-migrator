#!/usr/bin/env bash
# Fail if any Rust source file (excluding tests/sql) exceeds 100 lines of non-blank, non-comment code.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MAX=100
fail=0
while IFS= read -r f; do
  [[ "$f" == *"/tests/"* ]] && continue
  n=$(grep -v '^\s*$' "$f" | grep -v '^\s*//' | grep -v '^\s*#' | wc -l | tr -d ' ')
  if [[ "$n" -gt "$MAX" ]]; then
    echo "LOC $n > $MAX: $f"
    fail=1
  fi
done < <(find "$ROOT/rust/crates" -name '*.rs' -print)
exit "$fail"
