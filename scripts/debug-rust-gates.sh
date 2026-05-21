#!/usr/bin/env bash
# Debug gate runner: logs each rust-check step to .cursor/debug-622565.log (NDJSON).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOG="$ROOT/.cursor/debug-622565.log"
SESSION="622565"
RUN_ID="${1:-pre-fix}"

log() {
  local hyp="$1" loc="$2" msg="$3" data="${4:-{}}"
  printf '{"sessionId":"%s","runId":"%s","hypothesisId":"%s","location":"%s","message":"%s","data":%s,"timestamp":%s}\n' \
    "$SESSION" "$RUN_ID" "$hyp" "$loc" "$msg" "$data" "$(($(date +%s)*1000))" >>"$LOG"
}

run_step() {
  local hyp="$1" name="$2"
  shift 2
  log "$hyp" "debug-rust-gates.sh" "step_start" "{\"step\":\"$name\"}"
  if "$@" >/tmp/debug_rust_gate_out.txt 2>&1; then
    log "$hyp" "debug-rust-gates.sh" "step_pass" "{\"step\":\"$name\"}"
    return 0
  else
    local ec=$?
    local tail
    tail=$(tail -3 /tmp/debug_rust_gate_out.txt | tr '\n' ' ' | sed 's/"/\\"/g')
    log "$hyp" "debug-rust-gates.sh" "step_fail" "{\"step\":\"$name\",\"exit\":$ec,\"tail\":\"$tail\"}"
    return $ec
  fi
}

log "H0" "debug-rust-gates.sh" "run_start" "{\"root\":\"$ROOT\"}"

run_step H4 "fmt" bash -c "cd '$ROOT/rust' && cargo fmt --all -- --check" || true
run_step H3 "clippy" bash -c "cd '$ROOT/rust' && RUSTFLAGS='-D warnings' cargo clippy -p migrator-core -p rmig -p rmigd --all-targets -- -D warnings" || true
run_step H1 "cargo_test" bash -c "cd '$ROOT/rust' && RUSTFLAGS='-D warnings' cargo test -p migrator-core --lib --tests" || true
run_step H2 "rust_slo_makefile" bash -c "cd '$ROOT' && make rust-slo" || true

log "H0" "debug-rust-gates.sh" "run_end" "{}"
