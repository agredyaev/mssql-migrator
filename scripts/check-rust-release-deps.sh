#!/usr/bin/env bash
# Fail if profiler/bench-only crates appear in normal (release) dependency trees.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

FORBIDDEN='pprof|criterion|dhat|plotters|inferno|migrator-core-dev'
fail=0

check_pkg() {
  local pkg="$1"
  local tree hits
  # A failed cargo invocation must fail the gate: an empty tree is NOT proof
  # that no forbidden dependency exists.
  if ! tree="$(cargo tree -p "$pkg" -e normal)"; then
    echo "release-deps: cargo tree failed for $pkg" >&2
    exit 1
  fi
  hits="$(printf '%s\n' "$tree" | rg -i "$FORBIDDEN" || true)"
  if [[ -n "$hits" ]]; then
    echo "release-deps: $pkg normal dependency tree must not include dev-only crates:" >&2
    echo "$hits" >&2
    fail=1
  fi
}

check_pkg migrator-core
check_pkg rmig
check_pkg rmigd

if [[ -e "$ROOT/rust" ]]; then
  echo "release-deps: legacy rust/ directory must not exist (moved to repo-root crates/)" >&2
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
echo "release-deps: OK (no pprof/criterion/dhat in normal trees; no core-dev in production)"
