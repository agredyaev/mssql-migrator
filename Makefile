SHELL := /bin/bash

.PHONY: all build release-build test check doc-check doc-rust deny db-up db-down db-init \
	arch e2e e2e-all e2e-timings check-e2e sql-regression script-tests \
	slo prod-gate plan-db-perf workflow-fast \
	bench-footprint bench-footprint-profile bench-footprint-alloc \
	bench-footprint-scan \
	bench-footprint-update-baseline profile-summary test-int \
	bump bump-minor bump-major

RELEASE_BIN = bin/rmig

all: check

build:
	cargo build --release -p rmig -p rmigd

release-build:
	cargo build --profile release-dist -p rmig -p rmigd
	@mkdir -p $(dir $(RELEASE_BIN))
	cp -f target/release-dist/rmig $(RELEASE_BIN)

test:
	cargo test -p migrator-core --all-features --lib --tests
	cargo test -p rmig
	cargo test -p rmigd

deny:
	cargo deny check

arch:
	scripts/check-rust-arch.sh
	scripts/check-rust-release-deps.sh
	scripts/check-rust-release-profile.sh
	scripts/check-rust-loc.sh
	scripts/check-e2e-scenarios.sh
	scripts/check-prod-gate-reset.sh
	scripts/check-no-inline-sql.sh

script-tests:
	ops/quality/scripts/tests/run.sh

check: arch script-tests
	cargo fmt --all -- --check
	RUSTFLAGS="-D warnings" cargo clippy -p migrator-core -p rmig -p rmigd --lib --bins --tests -- -D warnings
	RUSTFLAGS="-D warnings" cargo test -p migrator-core --all-features --lib --tests
	RUSTDOCFLAGS="-D warnings" cargo doc --workspace --no-deps
	@echo "check: PASS (SQL gate: make sql-regression && make check-e2e)"

doc-check:
	python3 ops/quality/scripts/check_doc_structure.py
	python3 ops/quality/scripts/check_doc_context.py
	python3 ops/quality/scripts/check_doc_path_references.py
	python3 ops/quality/scripts/check_doc_language.py
	python3 ops/quality/scripts/check_doc_sync.py

doc-rust:
	RUSTDOCFLAGS="-D warnings" cargo doc --workspace --no-deps

db-up:
	docker compose up -d --wait
	@echo "Catalog databases are created on first rmig run (from directories under RM_SQL_ROOT)."

db-down:
	docker compose down -v

db-init: db-up

test-int: db-up
	@export ROOT="$(CURDIR)" && set -a && . ops/perf/e2e_env.sh && set +a && \
	RUSTFLAGS="-D warnings" cargo test --release -p migrator-core --test integration_plan -- --nocapture --test-threads=1

e2e: db-up
	ops/perf/e2e.sh $(ARGS)

e2e-all: db-up
	ops/perf/e2e_all.sh $(ARGS)

e2e-timings:
	python3 ops/perf/e2e_timings.py

sql-regression: db-up
	ops/perf/sql_regression.sh $(ARGS)

check-e2e: db-up
	@$(MAKE) sql-regression
	@$(MAKE) e2e-all
	@$(MAKE) workflow-fast
	@$(MAKE) slo
	@$(MAKE) prod-gate
	@echo "check-e2e: ALL PASS"

prod-gate: db-up
	ops/perf/prod_gate.sh $(ARGS)

slo: db-up
	RMIG_USE_RMIGD=1 ops/perf/cli_phase.sh slo $(ARGS)

plan-db-perf: db-up
	ops/perf/plan_db_perf.sh $(ARGS)

workflow-fast: db-up
	ops/perf/workflow_fast.sh $(ARGS)

bench-footprint:
	ops/perf/footprint_bench.sh bench $(ARGS)

bench-footprint-profile:
	ops/perf/footprint_bench.sh profile $(ARGS)

bench-footprint-alloc:
	ops/perf/footprint_bench.sh alloc $(ARGS)

bench-footprint-scan:
	ops/perf/footprint_bench.sh profile-load-scan $(ARGS)
	ops/perf/footprint_bench.sh alloc scan_root

bench-footprint-update-baseline:
	ops/perf/footprint_bench.sh update-baseline $(ARGS)

profile-summary:
	ops/perf/profile_summary.sh

full-check: check doc-check doc-rust

bump:
	python3 scripts/bump-version.py patch

bump-minor:
	python3 scripts/bump-version.py minor

bump-major:
	python3 scripts/bump-version.py major
