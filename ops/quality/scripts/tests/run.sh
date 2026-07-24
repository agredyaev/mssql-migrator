#!/usr/bin/env bash
# Regression tests for shell/python gates and harness scripts.
# Each case replays the reproduction of a fixed bug against the CURRENT
# scripts, so the gate itself has a gate. Offline; no Docker, no SQL.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
# ROOT resolves to repo root: tests/ -> scripts/ -> quality/ -> ops/ -> repo
[[ -f "$ROOT/Makefile" ]] || { echo "script-tests: bad ROOT $ROOT" >&2; exit 1; }

PASS=0
FAIL=0

ok()   { PASS=$((PASS + 1)); echo "ok   - $1"; }
bad()  { FAIL=$((FAIL + 1)); echo "FAIL - $1" >&2; }
check() { # check <name> <expected_rc> cmd...
  local name="$1" want="$2"; shift 2
  local rc=0
  "$@" >/dev/null 2>&1 || rc=$?
  if [[ "$rc" -eq "$want" ]]; then ok "$name"; else bad "$name (rc=$rc want=$want)"; fi
}

tmp_root() { mktemp -d "${TMPDIR:-/tmp}/rmig-script-tests.XXXXXX"; }

# --- bump-version: validate-first, table-bounded regex ---------------------
t="$(tmp_root)"
mkdir -p "$t/scripts"
cp "$ROOT/scripts/bump-version.py" "$t/scripts/"
cat > "$t/Cargo.toml" <<'EOF'
[workspace.package]
edition = "2021"

[unrelated]
version = "9.9.9"
EOF
rc=0; (cd "$t" && python3 scripts/bump-version.py patch) >/dev/null 2>&1 || rc=$?
if [[ "$rc" -ne 0 ]] && grep -q '9\.9\.9' "$t/Cargo.toml"; then
  ok "bump-version leaves Cargo.toml untouched when workspace version is missing"
else
  bad "bump-version validation (rc=$rc)"
fi
rm -rf "$t"

# --- release-deps: a failing cargo must fail the gate ----------------------
t="$(tmp_root)"
mkdir -p "$t/bin"
printf '#!/bin/sh\nexit 42\n' > "$t/bin/cargo"; chmod +x "$t/bin/cargo"
check "release-deps fails when cargo tree fails" 1 \
  env PATH="$t/bin:$PATH" bash "$ROOT/scripts/check-rust-release-deps.sh"
rm -rf "$t"

# --- release-profile: keys in unrelated tables must not satisfy the gate ---
t="$(tmp_root)"
mkdir -p "$t/scripts"
cp "$ROOT/scripts/check-rust-release-profile.sh" "$t/scripts/"
cat > "$t/Cargo.toml" <<'EOF'
[profile.release-dist]
inherits = "release"

[profile.unused]
lto = "fat"
strip = true
codegen-units = 1
debug = false
panic = "abort"
incremental = false

[profile.release]
debug = true
EOF
check "release-profile rejects keys parked in an unrelated table" 1 \
  bash "$t/scripts/check-rust-release-profile.sh"
rm -rf "$t"

# --- check-rust-arch: nested CLI modules are scanned -----------------------
t="$(tmp_root)"
mkdir -p "$t/scripts" "$t/crates/cli/src/nested" "$t/crates/rmigd/src" "$t/crates/core/src"
cp "$ROOT/scripts/check-rust-arch.sh" "$t/scripts/"
echo 'use migrator_core::session::connect_daemon;' > "$t/crates/cli/src/nested/forbidden.rs"
cat > "$t/crates/rmigd/src/main.rs" <<'EOF'
use migrator_core::session;
fn main() { migrator_core::session::run_daemon(); }
EOF
check "arch gate catches forbidden imports in nested CLI modules" 1 \
  bash "$t/scripts/check-rust-arch.sh"
rm -rf "$t"

# --- check-e2e-scenarios: gutted orchestrator must fail --------------------
t="$(tmp_root)"
mkdir -p "$t/scripts" "$t/crates/core/tests/testdata/e2e" "$t/ops/perf"
cp "$ROOT/scripts/check-e2e-scenarios.sh" "$t/scripts/"
for s in empty_db_plan warm_db_plan skip_unchanged_plan catalog_cache_plan; do
  echo '{}' > "$t/crates/core/tests/testdata/e2e/e2e_baseline_${s}.json"
done
# Names only in a COMMENT; orchestrator invokes nothing.
cat > "$t/crates/core/tests/scenario_e2e_integration.rs" <<'EOF'
// empty_db_plan warm_db_plan skip_unchanged_plan catalog_cache_plan
EOF
echo 'echo matrix removed' > "$t/ops/perf/e2e_all.sh"
check "e2e-scenarios gate fails when the orchestrator invokes nothing" 1 \
  bash "$t/scripts/check-e2e-scenarios.sh"
rm -rf "$t"

# --- doc path gate: crates/ refs are existence-checked ---------------------
t="$(tmp_root)"
mkdir -p "$t/docs" "$t/ops/quality/scripts"
cp "$ROOT"/ops/quality/scripts/*.py "$t/ops/quality/scripts/"
printf 'See `crates/does-not-exist.rs` for details.\n' > "$t/docs/probe.md"
check "doc path gate fails on a missing crates/ path" 1 \
  env REPO_ROOT="$t" python3 "$t/ops/quality/scripts/check_doc_path_references.py"
rm -rf "$t"

# --- doc structure/context: fenced examples cannot satisfy the contract ----
t="$(tmp_root)"
mkdir -p "$t/docs" "$t/ops/quality/scripts"
cp "$ROOT"/ops/quality/scripts/*.py "$t/ops/quality/scripts/"
cat > "$t/docs/fenced.md" <<'EOF'
# Fenced probe

```markdown
Lifecycle: `Current`.

## Purpose
## Scope
## System Context
## Interfaces And Boundaries
## Assumptions And Constraints
## Nominal Flow
## Off-Nominal Behavior And Failure Containment
## Verification And Validation
## Operations And Recovery
## Open Issues And Non-Goals
## References
```
EOF
check "doc structure gate ignores headings inside fences" 1 \
  env REPO_ROOT="$t" python3 "$t/ops/quality/scripts/check_doc_structure.py"
check "doc context gate ignores lifecycle inside fences" 1 \
  env REPO_ROOT="$t" python3 "$t/ops/quality/scripts/check_doc_context.py"
rm -rf "$t"

# --- doc sync: a prose TODO is not an index entry --------------------------
t="$(tmp_root)"
mkdir -p "$t/docs/specs/rust" "$t/ops/quality/scripts"
cp "$ROOT"/ops/quality/scripts/*.py "$t/ops/quality/scripts/"
touch "$t/docs/specs/rust/module-hidden.md"
cat > "$t/docs/specs/rust/README.md" <<'EOF'
# Index

TODO: `module-hidden.md` must be added to the index.
EOF
check "doc sync gate rejects filename mentions outside index rows/links" 1 \
  env REPO_ROOT="$t" python3 "$t/ops/quality/scripts/check_doc_sync.py"
rm -rf "$t"

# --- e2e_timings: typed deltas, no ms arithmetic on strings/bools ----------
if python3 - "$ROOT" <<'EOF'
import sys
sys.path.insert(0, sys.argv[1] + "/ops/perf")
import e2e_timings as t
assert t.delta_str("cold_full", "git_delta") == "cold_full -> git_delta"
assert t.delta_str("cold_full", "cold_full") == "="
assert "ms" not in t.delta_str(False, True), t.delta_str(False, True)
assert t.delta_str(100, 150).startswith("+50ms")
EOF
then ok "e2e_timings renders categorical/bool deltas as transitions"; else bad "e2e_timings delta typing"; fi

# --- e2e_timings: a missing scenario must fail, not report green ------------
t="$(tmp_root)"
mkdir -p "$t/ops/perf/artifacts" "$t/crates/core/tests/testdata/e2e"
cp "$ROOT/ops/perf/e2e_timings.py" "$t/ops/perf/"
for s in empty_db_plan warm_db_plan skip_unchanged_plan catalog_cache_plan; do
  echo '{"timings":{"plan_wall_ms":1}}' > "$t/crates/core/tests/testdata/e2e/e2e_baseline_${s}.json"
done
for s in empty_db_plan warm_db_plan skip_unchanged_plan; do
  echo '{"timings":{"plan_wall_ms":1}}' > "$t/ops/perf/artifacts/e2e_${s}.json"
done
check "e2e_timings fails when a required scenario artifact is missing" 1 \
  python3 "$t/ops/perf/e2e_timings.py"
rm -rf "$t"

# --- dhat_alloc_tree: zero iterations rejected at the CLI ------------------
t="$(tmp_root)"
echo '{}' > "$t/heap.json"
check "dhat_alloc_tree rejects --iterations 0" 2 \
  python3 "$ROOT/ops/perf/dhat_alloc_tree.py" "$t/heap.json" --iterations 0
rm -rf "$t"

# --- dhat phase attribution: loop markers stay in sync with bench frames ----
if grep -q 'LOOP_MARKERS = ("bench_loop",)' "$ROOT/ops/perf/dhat_alloc_tree.py" \
    && grep -q 'fn bench_loop' "$ROOT/crates/core-dev/benches/scan_dhat.rs" \
    && grep -q 'inline(never)' "$ROOT/crates/core-dev/benches/scan_dhat.rs"; then
  ok "dhat LOOP_MARKERS match inline(never) bench_loop frames"
else
  bad "dhat loop marker sync"
fi

# --- perf summary: committed evidence must be repo-relative & tracked -------
t="$(tmp_root)"
mkdir -p "$t/artifacts"
touch "$t/artifacts/plan_diff_5k_flamegraph.svg" \
  "$t/artifacts/rust_plan_diff_5k_flamegraph.svg"
# shellcheck source=ops/perf/profile_identity.sh
source "$ROOT/ops/perf/profile_identity.sh"
profile_id="$(rmig_profile_identity "$ROOT")"
printf '# %s\nartifact: %s/target/release/rmig\n' "$profile_id" "$ROOT" \
  > "$t/artifacts/rust_plan_diff_dhat.txt"
printf '\n<!-- %s -->\n' "$profile_id" \
  >> "$t/artifacts/rust_plan_diff_5k_flamegraph.svg"
RMIG_FOOTPRINT_ARTIFACTS="$t/artifacts" bash "$ROOT/ops/perf/profile_summary.sh" >/dev/null
if grep -q 'rust_plan_diff_5k_flamegraph.svg' "$t/artifacts/profile_summary.txt" \
    && ! grep -q '  ops/perf/artifacts/plan_diff_5k_flamegraph.svg' "$t/artifacts/profile_summary.txt" \
    && ! grep -Fq "$ROOT/" "$t/artifacts/profile_summary.txt"; then
  ok "profile summary prefers tracked Rust artifacts and scrubs checkout paths"
else
  bad "profile summary generation portability"
fi
sed '1s/.*/# stale-profile/' "$t/artifacts/rust_plan_diff_dhat.txt" \
  > "$t/artifacts/stale.txt"
mv "$t/artifacts/stale.txt" "$t/artifacts/rust_plan_diff_dhat.txt"
check "profile summary rejects stale artifact identity" 1 \
  env RMIG_FOOTPRINT_ARTIFACTS="$t/artifacts" \
  bash "$ROOT/ops/perf/profile_summary.sh"
rm -rf "$t"

if ! grep -q '/Users/' "$ROOT/ops/perf/artifacts/profile_summary.txt" \
    && ! grep -q 'artifacts/plan_diff_5k_flamegraph.svg' "$ROOT/ops/perf/artifacts/profile_summary.txt"; then
  ok "profile summary references tracked repo-relative artifacts"
else
  bad "profile summary references untracked/absolute paths"
fi

# --- e2e_all finalizer preserves scenario rows (no same-path tee) ----------
if grep -q 'mktemp' "$ROOT/ops/perf/e2e_all.sh" \
    && ! grep -q 'cat "\$REPORT"$' <(grep -A2 'e2e ALL: PASS' "$ROOT/ops/perf/e2e_all.sh" | grep 'tee'); then
  ok "e2e_all finalizes via temp+rename, not a same-path tee"
else
  bad "e2e_all finalizer structure"
fi

# --- sql_regression: injection guard + identity-checked lock/cleanup -------
if grep -q 'A-Za-z0-9_' "$ROOT/ops/perf/sql_regression.sh" \
    && grep -q 'lstart' "$ROOT/ops/perf/sql_regression.sh" \
    && grep -q 'RMIGD_SOCKET must live under' "$ROOT/ops/perf/sql_regression.sh" \
    && ! grep -q 'pkill -f' "$ROOT/ops/perf/sql_regression.sh"; then
  ok "sql_regression guards db names, lock identity, and socket cleanup"
else
  bad "sql_regression guard structure"
fi

# --- mutating plan: every managed object must be live-inspected ------------
if grep -q 'ctx.full || ctx.bypass' \
    "$ROOT/crates/core/src/db/plan_common/body/standard/mod.rs"; then
  ok "mutating plan bypass forces a full managed-object inspection"
else
  bad "mutating plan can fall back to deterministic spot checks"
fi

# --- raw SQL setup steps must use the command timeout ----------------------
if grep -q '"session init", init_session' "$ROOT/crates/core/src/driver/mssql.rs" \
    && grep -q 'with_create_database_timeout(t, db, mssql::exec' \
      "$ROOT/crates/core/src/config/ensure_db.rs"; then
  ok "session init and CREATE DATABASE call sites are timeout-bounded"
else
  bad "raw SQL setup timeout call sites"
fi

# --- release workflow: source, evidence, and publication invariants --------
if grep -Fq 'REF_TYPE: ${{ github.ref_type }}' "$ROOT/.github/workflows/release.yml" \
    && grep -Fq 'main|master)' "$ROOT/.github/workflows/release.yml" \
    && grep -q 'steps.release_state.outputs.validated_sha' "$ROOT/.github/workflows/release.yml" \
    && grep -q 'workflow_run.head_branch || github.ref_name' "$ROOT/.github/workflows/release.yml" \
    && grep -Fq -- '--event push --branch "$RELEASE_BRANCH" --status success' "$ROOT/.github/workflows/release.yml" \
    && ! grep -Fq 'branch="${{' "$ROOT/.github/workflows/release.yml" \
    && grep -q 'git push --atomic' "$ROOT/.github/workflows/release.yml" \
    && grep -q 'steps.release_state.outputs.resume' "$ROOT/.github/workflows/release.yml"; then
  ok "release workflow binds protected branch, push-CI SHA, atomic refs, and resume path"
else
  bad "release workflow invariants"
fi

# claim_lock runtime: an unrelated LIVE pid must not impersonate the owner
t="$(tmp_root)"
sleep 60 & DUMMY=$!
mkdir -p "$t/.rmig/sql-regression.lock"
printf '%s\nnot-the-real-start-time\n' "$DUMMY" > "$t/.rmig/sql-regression.lock/pid"
if env ROOT="$t" bash -c '
  set -euo pipefail
  LOCK_DIR="$ROOT/.rmig/sql-regression.lock"
  LOCK_PID="$LOCK_DIR/pid"
  '"$(sed -n '/^proc_start()/,/^}/p;/^write_lock_owner()/,/^}/p;/^claim_lock()/,/^}/p' "$ROOT/ops/perf/sql_regression.sh")"'
  claim_lock
' >/dev/null 2>&1; then
  ok "claim_lock reclaims a lock held by a reused unrelated PID"
else
  bad "claim_lock PID-reuse reclamation"
fi
kill "$DUMMY" 2>/dev/null || true
rm -rf "$t"

# --- sql_regression: remote bootstrap target must be refused ----------------
t="$(tmp_root)"
mkdir -p "$t/sqlroot"
check "sql_regression refuses a remote bootstrap target" 1 \
  env RM_DB_SERVER=db.remote.example RM_SQL_ROOT="$t/sqlroot" bash "$ROOT/ops/perf/sql_regression.sh"
rm -rf "$t"

# --- destructive SQL harnesses must stay on loopback ------------------------
check "e2e env refuses a remote SQL target" 1 \
  env ROOT="$ROOT" RM_DB_SERVER=db.remote.example bash -c \
  'source "$ROOT/ops/perf/e2e_env.sh"'

t="$(tmp_root)"
mkdir -p "$t/ops/perf" "$t/bin" "$t/sqlroot/dactests/smoke"
cp "$ROOT/ops/perf/prod_gate.sh" "$t/ops/perf/"
printf '#!/bin/sh\nexit 0\n' > "$t/bin/cargo"; chmod +x "$t/bin/cargo"
printf '#!/bin/sh\nexit 99\n' > "$t/bin/docker"; chmod +x "$t/bin/docker"
check "prod gate refuses a remote database reset before Docker" 1 \
  env PATH="$t/bin:$PATH" RM_DB_SERVER=db.remote.example RM_SQL_ROOT="$t/sqlroot" \
  bash "$t/ops/perf/prod_gate.sh"
check "prod gate permits an explicit remote no-reset plan" 0 \
  env PATH="$t/bin:$PATH" RM_DB_SERVER=db.remote.example RM_SQL_ROOT="$t/sqlroot" \
  RMIG_GATE_SKIP_DB_RESET=1 bash "$t/ops/perf/prod_gate.sh"
rm -rf "$t"

if grep -q 'SQLCMDPASSWORD' "$ROOT/ops/perf/sql_regression.sh" \
    && grep -q 'SQLCMDPASSWORD' "$ROOT/ops/perf/prod_gate.sh" \
    && grep -q 'SQLCMDPASSWORD' "$ROOT/docker-compose.yml" \
    && ! grep -q -- '-P' "$ROOT/ops/perf/sql_regression.sh" \
    && ! grep -q -- '-P' "$ROOT/ops/perf/prod_gate.sh" \
    && ! grep -q -- '-P' "$ROOT/docker-compose.yml"; then
  ok "sqlcmd passwords stay out of process arguments"
else
  bad "sqlcmd password environment wiring"
fi

# --- footprint_bench alloc: stale heap must not be republished -------------
t="$(tmp_root)"
mkdir -p "$t/ops/perf/artifacts" "$t/bin" "$t/crates/core-dev"
cp "$ROOT/ops/perf/footprint_bench.sh" "$ROOT/ops/perf/profile_identity.sh" "$t/ops/perf/"
printf '[workspace.package]\nversion = "test"\n' > "$t/Cargo.toml"
echo '{"old": true}' > "$t/ops/perf/artifacts/dhat_heap.json"
printf '#!/bin/sh\nexit 0\n' > "$t/bin/cargo"; chmod +x "$t/bin/cargo"
printf '#!/bin/sh\nexit 0\n' > "$t/bin/python3"; chmod +x "$t/bin/python3"
check "footprint alloc fails without a FRESH dhat heap" 1 \
  env PATH="$t/bin:$PATH" bash "$t/ops/perf/footprint_bench.sh" alloc skip_heavy
if [[ ! -f "$t/ops/perf/artifacts/dhat_heap.json" ]]; then
  ok "footprint alloc removed the stale heap instead of republishing it"
else
  bad "footprint alloc stale heap still present"
fi
rm -rf "$t"

# --- footprint_bench profile: stale criterion flamegraph must not pass ------
t="$(tmp_root)"
mkdir -p "$t/ops/perf/artifacts" "$t/bin" "$t/target/criterion/plan_diff_skip_heavy_5000/profile"
cp "$ROOT/ops/perf/footprint_bench.sh" "$ROOT/ops/perf/profile_identity.sh" "$t/ops/perf/"
printf '[workspace.package]\nversion = "test"\n' > "$t/Cargo.toml"
echo '<svg/>' > "$t/target/criterion/plan_diff_skip_heavy_5000/profile/flamegraph.svg"
printf '#!/bin/sh\nexit 0\n' > "$t/bin/cargo"; chmod +x "$t/bin/cargo"
check "footprint profile fails without a FRESH flamegraph" 1 \
  env PATH="$t/bin:$PATH" bash "$t/ops/perf/footprint_bench.sh" profile
if [[ ! -e "$t/target/criterion/plan_diff_skip_heavy_5000/profile/flamegraph.svg" ]]; then
  ok "footprint profile wiped stale criterion output before benching"
else
  bad "footprint profile kept stale criterion output"
fi
rm -rf "$t"

# --- plan_db_perf: success requires a fresh trace --------------------------
t="$(tmp_root)"
mkdir -p "$t/ops/perf/artifacts" "$t/bin"
cp "$ROOT/ops/perf/plan_db_perf.sh" "$t/ops/perf/"
printf '#!/bin/sh\nexit 0\n' > "$t/bin/cargo"; chmod +x "$t/bin/cargo"
check "plan_db_perf fails when no trace artifact was produced" 1 \
  env PATH="$t/bin:$PATH" RMIG_PLAN_DB_TRACE=0 bash "$t/ops/perf/plan_db_perf.sh"
rm -rf "$t"

# --- compose/Makefile invariants -------------------------------------------
if grep -q 'SQLCMDPASSWORD=.*\$\$MSSQL_SA_PASSWORD' "$ROOT/docker-compose.yml" \
    && grep -q '127\.0\.0\.1:' "$ROOT/docker-compose.yml" \
    && ! grep -q -- '-P' "$ROOT/docker-compose.yml" \
    && ! grep -q 'container_name:' "$ROOT/docker-compose.yml"; then
  ok "compose: healthcheck env password, loopback bind, no fixed container name"
else
  bad "compose invariants"
fi

# db-up: an unhealthy server (compose up --wait) must fail the target
t="$(tmp_root)"
mkdir -p "$t/ops/perf" "$t/bin"
cp "$ROOT/Makefile" "$t/"
cp "$ROOT/ops/perf/e2e_env.sh" "$t/ops/perf/"
cat > "$t/bin/docker" <<'EOF'
#!/bin/sh
case "$1 $2" in
  "compose up") exit 1 ;;
esac
exit 0
EOF
chmod +x "$t/bin/docker"
check "make db-up fails when SQL Server never becomes ready" 2 \
  env PATH="$t/bin:/usr/bin:/bin" make -C "$t" db-up
rm -rf "$t"

echo ""
echo "script-tests: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
