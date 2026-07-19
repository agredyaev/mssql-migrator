#!/usr/bin/env bash
# Full CLI CPU flamegraph (requires: cargo install flamegraph).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
mkdir -p "$ROOT/ops/perf/artifacts"

if ! command -v flamegraph >/dev/null 2>&1; then
  echo "install: cargo install flamegraph" >&2
  exit 1
fi

export CARGO_PROFILE_RELEASE_DEBUG=true
export RM_SKIP_GIT="${RM_SKIP_GIT:-1}"
export RM_DB_SERVER="${RM_DB_SERVER:-127.0.0.1}"
export RM_DB_PORT="${RM_DB_PORT:-1433}"
export RM_DB_USER="${RM_DB_USER:-sa}"
export RM_DB_PASSWORD="${RM_DB_PASSWORD:-yourStrong(!)Password}"
export RM_DB_ENCRYPT="${RM_DB_ENCRYPT:-false}"
export RM_DB_TRUST_SERVER_CERTIFICATE="${RM_DB_TRUST_SERVER_CERTIFICATE:-true}"
export RM_SQL_ROOT="${RM_SQL_ROOT:-$ROOT/.temp/sql}"
export RM_SQL_BASE="${RM_SQL_BASE:-$RM_SQL_ROOT}"

OUT="$ROOT/ops/perf/artifacts/rmig_plan_flamegraph.svg"
flamegraph -o "$OUT" -p rmig -- plan
echo "wrote $OUT"
