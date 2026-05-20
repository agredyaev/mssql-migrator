#!/usr/bin/env bash
# Fail if profiler/bench-only crates appear in normal (release) dependency trees.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/rust"

FORBIDDEN='pprof|criterion|dhat|plotters|inferno'
fail=0

check_pkg() {
  local pkg="$1"
  local hits
  hits="$(cargo tree -p "$pkg" -e normal 2>/dev/null | rg -i "$FORBIDDEN" || true)"
  if [[ -n "$hits" ]]; then
    echo "release-deps: $pkg normal dependency tree must not include dev-only crates:" >&2
    echo "$hits" >&2
    fail=1
  fi
}

check_pkg migrator-core
check_pkg rmig
check_pkg rmigd

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "release-deps: OK (no pprof/criterion/dhat in normal trees)"
