#!/usr/bin/env bash
# Fail if profiler/bench-only crates appear in normal (release) dependency trees.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

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

for pkg in rmig rmigd migrator-core; do
  if cargo tree -p "$pkg" -e normal 2>/dev/null | rg -q 'migrator-core-dev'; then
    echo "release-deps: $pkg must not depend on migrator-core-dev (benches/footprint only)" >&2
    fail=1
  fi
done

if [[ -e "$ROOT/rust" ]]; then
  echo "release-deps: legacy rust/ directory must not exist (moved to repo-root crates/)" >&2
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "release-deps: OK (no pprof/criterion/dhat in normal trees; no core-dev in production)"
