.PHONY: all build test vet lint fmt check doc-check release-build db-up db-down db-init test-int test-int-phase test-int-phase-cli test-int-phase-cli-warm test-cli-phase-cold test-cli-phase-warm test-prod-gate test-prod-gate-update-baseline go-rust-e2e go-rust-e2e-all check-e2e go-rust-io-debug rust-check rust-e2e rust-prod-gate rust-slo rust-plan-db-perf rust-workflow-fast bench-footprint bench-footprint-profile bench-footprint-update-baseline profile-summary test-int-v

STATICCHECK = $(shell go env GOPATH)/bin/staticcheck

# Production-oriented flags (trim paths, strip debug/symbol tables, pure Go link).
# Compiler chapter (inlining, escape analysis, build flags):
#   https://psavelis.github.io/golang-performance-optimization/optimization/compiler/
# Concrete flag set follows "Build optimization" in that chapter:
#   https://psavelis.github.io/golang-performance-optimization/optimization/compiler/build-optimization.html
# Note: -extldflags=-static is omitted by default (often problematic on macOS / libc DNS).
RELEASE_BIN = bin/rmig

# Semver for release binaries (root VERSION); commit from git for -ldflags.
RELEASE_VERSION := $(shell cat VERSION 2>/dev/null | tr -d '\n' || echo 0.0.0-dev)
RELEASE_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
RELEASE_LDFLAGS = -s -w \
	-X reporting-db-migrations/internal/buildinfo.Version=$(RELEASE_VERSION) \
	-X reporting-db-migrations/internal/buildinfo.Commit=$(RELEASE_COMMIT)

all: check

build:
	go build ./...

# Optimized CLI binary; compiles the full module graph for ./cmd/rmig.
release-build:
	@mkdir -p $(dir $(RELEASE_BIN))
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="$(RELEASE_LDFLAGS)" -o $(RELEASE_BIN) ./cmd/rmig

test:
	go test ./...

vet:
	go vet ./...

lint:
	$(STATICCHECK) ./...

fmt:
	test -z "$$(gofmt -l .)"

check: build test vet lint fmt

doc-check:
	python3 ops/quality/scripts/check_doc_structure.py
	python3 ops/quality/scripts/check_doc_context.py
	python3 ops/quality/scripts/check_doc_path_references.py
	python3 ops/quality/scripts/check_doc_language.py
	python3 ops/quality/scripts/check_doc_sync.py

db-up:
	docker compose up -d
	@echo "Waiting for MSSQL to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		docker compose exec -T mssql /opt/mssql-tools18/bin/sqlcmd \
			-S localhost -U sa -P 'yourStrong(!)Password' -C \
			-Q "SELECT 1" >/dev/null 2>&1 && break; \
		sleep 3; \
	done
	@echo "Catalog databases are created on first rmig run (from directories under RM_SQL_ROOT)."

db-down:
	docker compose down -v

db-init: db-up

test-int:
	RMIG_RUN_SQLSERVER_INTEGRATION=1 \
	RM_DB_SERVER=localhost \
	RM_DB_PORT=1433 \
	RM_DB_DATABASE=rmig_test \
	RM_DB_USER=sa \
	RM_DB_PASSWORD='yourStrong(!)Password' \
	RM_DB_ENCRYPT=false \
	RM_DB_TRUST_SERVER_CERTIFICATE=true \
	go test -tags=integration ./internal/app/ -run TestIntegration -v -count=1 $(ARGS)

# Phase timings + driver.Conn boundary (Query/Exec/Ping); optional profiles, e.g.
#   make test-int-phase ARGS='-cpuprofile=/tmp/rmig.cpu.prof -memprofile=/tmp/rmig.mem.prof -trace=/tmp/rmig.trace'
test-int-phase:
	RMIG_RUN_SQLSERVER_INTEGRATION=1 \
	RM_DB_SERVER=localhost \
	RM_DB_PORT=1433 \
	RM_DB_DATABASE=rmig_test \
	RM_DB_USER=sa \
	RM_DB_PASSWORD='yourStrong(!)Password' \
	RM_DB_ENCRYPT=false \
	RM_DB_TRUST_SERVER_CERTIFICATE=true \
	go test -tags=integration ./internal/app/ -run TestIntegration_PhaseReport -v -count=1 $(ARGS)

# Full CLI phase timings (runWithLookup plan + migrate); optional pprof/trace via ARGS.
test-int-phase-cli:
	RMIG_RUN_SQLSERVER_INTEGRATION=1 \
	RM_DB_SERVER=localhost \
	RM_DB_PORT=1433 \
	RM_DB_DATABASE=rmig_test \
	RM_DB_USER=sa \
	RM_DB_PASSWORD='yourStrong(!)Password' \
	RM_DB_ENCRYPT=false \
	RM_DB_TRUST_SERVER_CERTIFICATE=true \
	go test -tags=integration ./internal/app/ -run TestIntegration_PhaseReport_CLI -v -count=1 $(ARGS)

# Full CLI plan only, warm DB (no DROP/CREATE). See ops/perf/cli_phase.sh.
test-int-phase-cli-warm:
	@chmod +x ops/perf/cli_phase.sh
	RMIG_PHASE_SKIP_DB_RESET=1 ops/perf/cli_phase.sh warm $(ARGS)

test-cli-phase-cold:
	@chmod +x ops/perf/cli_phase.sh
	ops/perf/cli_phase.sh cold $(ARGS)

test-cli-phase-warm:
	@chmod +x ops/perf/cli_phase.sh
	ops/perf/cli_phase.sh warm $(ARGS)

# Incremental prod go/no-go: plan snapshot vs baseline, delta from git/env (see docs/prod-gate.md).
test-prod-gate:
	@chmod +x ops/perf/prod_gate.sh
	ops/perf/prod_gate.sh $(ARGS)

# In-process footprint baseline (struct sizes + diff benches); no SQL Server. See ops/perf/footprint_bench.sh.
bench-footprint:
	@chmod +x ops/perf/footprint_bench.sh
	ops/perf/footprint_bench.sh bench $(ARGS)

bench-footprint-profile:
	@chmod +x ops/perf/footprint_bench.sh
	ops/perf/footprint_bench.sh profile $(ARGS)

bench-footprint-update-baseline:
	@chmod +x ops/perf/footprint_bench.sh
	ops/perf/footprint_bench.sh update-baseline $(ARGS)

# Text top-20 from existing *.prof in ops/perf/artifacts (footprint + cli if present).
profile-summary:
	@chmod +x ops/perf/profile_summary.sh
	ops/perf/profile_summary.sh

# Rewrite internal/app/testdata/prod_gate/plan_baseline_empty_db.json after intentional plan changes.
test-prod-gate-update-baseline:
	RMIG_RUN_SQLSERVER_INTEGRATION=1 \
	RMIG_GATE_UPDATE_BASELINE=1 \
	RM_DB_SERVER=localhost \
	RM_DB_PORT=1433 \
	RM_DB_DATABASE=rmig_test \
	RM_DB_USER=sa \
	RM_DB_PASSWORD='yourStrong(!)Password' \
	RM_DB_ENCRYPT=false \
	RM_DB_TRUST_SERVER_CERTIFICATE=true \
	go test -tags=integration ./internal/app/ -run TestProdGate_IncrementalPlan -v -count=1

test-int-v: ARGS="-count=1"
test-int-v: test-int

# Go↔Rust plan snapshot parity on .temp/sql (see ops/perf/go_rust_e2e.sh).
go-rust-e2e: db-up
	@chmod +x ops/perf/go_rust_e2e.sh
	ops/perf/go_rust_e2e.sh $(ARGS)

# Full Go↔Rust e2e matrix (plan + apply + gate + blocked).
go-rust-e2e-all: db-up
	@chmod +x ops/perf/go_rust_e2e_all.sh
	ops/perf/go_rust_e2e_all.sh $(ARGS)

check-e2e: db-up
	@$(MAKE) go-rust-e2e-all
	@$(MAKE) go-rust-io-debug
	@$(MAKE) rust-workflow-fast
	@$(MAKE) rust-slo
	@$(MAKE) rust-prod-gate
	@echo "check-e2e: ALL PASS"

go-rust-io-debug: db-up
	@chmod +x ops/perf/go_rust_io_debug.sh
	ops/perf/go_rust_io_debug.sh $(ARGS)

rust-check:
	@chmod +x scripts/check-rust-arch.sh scripts/check-rust-release-deps.sh
	scripts/check-rust-arch.sh
	scripts/check-rust-release-deps.sh
	cd rust && cargo fmt --all -- --check
	cd rust && RUSTFLAGS="-D warnings" cargo clippy -p migrator-core -p rmig -p rmigd --all-targets -- -D warnings
	cd rust && RUSTFLAGS="-D warnings" cargo test -p migrator-core --lib --tests
	@echo "rust-check: PASS (integration SQL tests: make rust-e2e)"

rust-prod-gate: db-up
	@chmod +x ops/perf/rust_prod_gate.sh
	ops/perf/rust_prod_gate.sh $(ARGS)

rust-slo: db-up
	@chmod +x ops/perf/rust_cli_phase.sh
	RMIG_USE_RMIGD=1 ops/perf/rust_cli_phase.sh cold $(ARGS)

rust-plan-db-perf: db-up
	@chmod +x ops/perf/rust_plan_db_perf.sh
	ops/perf/rust_plan_db_perf.sh $(ARGS)

rust-workflow-fast: db-up
	@chmod +x ops/perf/rust_workflow_fast.sh
	ops/perf/rust_workflow_fast.sh $(ARGS)

# Rust-only SQL Server e2e: cold apply + full git workflow (no Go).
rust-e2e: db-up
	@chmod +x ops/perf/rust_e2e.sh
	ops/perf/rust_e2e.sh $(ARGS)
