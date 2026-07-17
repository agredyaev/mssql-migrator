SHELL := /bin/bash

.PHONY: all build release-build test check doc-check doc-rust db-up db-down db-init \
	arch e2e e2e-all e2e-timings check-e2e sql-regression integration script-tests \
	slo prod-gate plan-db-perf workflow-fast \
	bench-footprint bench-footprint-profile bench-footprint-alloc \
	bench-footprint-scan bench-footprint-cache \
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
	cargo test -p migrator-core --lib --tests
	cargo test -p rmigd

arch:
	@chmod +x scripts/check-rust-arch.sh scripts/check-rust-release-deps.sh scripts/check-rust-release-profile.sh scripts/check-rust-loc.sh scripts/check-e2e-scenarios.sh scripts/check-prod-gate-reset.sh scripts/check-e2e-git-flag.sh scripts/check-rm-db-database-contract.sh scripts/check-advisory-lock-release.sh scripts/check-sql-regression-manifest.sh ops/perf/sql_regression.sh
	scripts/check-rust-arch.sh
	scripts/check-rust-release-deps.sh
	scripts/check-rust-release-profile.sh
	scripts/check-rust-loc.sh
	scripts/check-e2e-scenarios.sh
	scripts/check-prod-gate-reset.sh
	scripts/check-e2e-git-flag.sh
	scripts/check-rm-db-database-contract.sh
	scripts/check-advisory-lock-release.sh
	scripts/check-sql-regression-manifest.sh

script-tests:
	@chmod +x ops/quality/scripts/tests/run.sh
	ops/quality/scripts/tests/run.sh

check: arch script-tests
	cargo fmt --all -- --check
	RUSTFLAGS="-D warnings" cargo clippy -p migrator-core -p rmig -p rmigd --lib --bins --tests -- -D warnings
	RUSTFLAGS="-D warnings" cargo test -p migrator-core --lib --tests
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
	docker compose up -d
	@echo "Waiting for MSSQL to be ready..."
	@export ROOT="$(CURDIR)" && set -a && . ops/perf/e2e_env.sh && set +a && \
	ready=0; \
	for i in 1 2 3 4 5 6 7 8 9 10; do \
		docker compose exec -T mssql /opt/mssql-tools18/bin/sqlcmd \
			-S "$$RM_DB_SERVER" -U "$$RM_DB_USER" -P "$$RM_DB_PASSWORD" -C \
			-Q "SELECT 1" >/dev/null 2>&1 && { ready=1; break; }; \
		sleep 3; \
	done; \
	if [ "$$ready" -ne 1 ]; then \
		echo "ERROR: MSSQL did not become ready after 10 probes" >&2; \
		exit 1; \
	fi
	@echo "Catalog databases are created on first rmig run (from directories under RM_SQL_ROOT)."

db-down:
	docker compose down -v

db-init: db-up

test-int: db-up
	RUSTFLAGS="-D warnings" cargo test --release -p migrator-core --test integration_plan -- --nocapture --test-threads=1

e2e: db-up
	@chmod +x ops/perf/e2e.sh
	ops/perf/e2e.sh $(ARGS)

e2e-all: db-up
	@chmod +x ops/perf/e2e_all.sh
	ops/perf/e2e_all.sh $(ARGS)

e2e-timings:
	python3 ops/perf/e2e_timings.py

integration: db-up
	@chmod +x ops/perf/integration.sh
	ops/perf/integration.sh $(ARGS)

sql-regression: db-up
	@chmod +x ops/perf/sql_regression.sh
	ops/perf/sql_regression.sh $(ARGS)

check-e2e: db-up
	@$(MAKE) sql-regression
	@$(MAKE) e2e-all
	@$(MAKE) workflow-fast
	@$(MAKE) slo
	@$(MAKE) prod-gate
	@echo "check-e2e: ALL PASS"

prod-gate: db-up
	@chmod +x ops/perf/prod_gate.sh
	ops/perf/prod_gate.sh $(ARGS)

slo: db-up
	@chmod +x ops/perf/cli_phase.sh
	@printf 'make slo debug session=1200a9 run_id=%s\n' "$${RMIG_DEBUG_RUN_ID:-manual}" >&2
	RMIG_USE_RMIGD=1 ops/perf/cli_phase.sh slo $(ARGS)

plan-db-perf: db-up
	@chmod +x ops/perf/plan_db_perf.sh
	ops/perf/plan_db_perf.sh $(ARGS)

workflow-fast: db-up
	@chmod +x ops/perf/workflow_fast.sh
	ops/perf/workflow_fast.sh $(ARGS)

bench-footprint:
	@chmod +x ops/perf/footprint_bench.sh
	ops/perf/footprint_bench.sh bench $(ARGS)

bench-footprint-profile:
	@chmod +x ops/perf/footprint_bench.sh
	ops/perf/footprint_bench.sh profile $(ARGS)

bench-footprint-alloc:
	@chmod +x ops/perf/footprint_bench.sh
	ops/perf/footprint_bench.sh alloc $(ARGS)

bench-footprint-scan:
	@chmod +x ops/perf/footprint_bench.sh
	ops/perf/footprint_bench.sh profile-load-scan $(ARGS)
	ops/perf/footprint_bench.sh alloc scan_root

bench-footprint-cache:
	@chmod +x ops/perf/footprint_bench.sh
	ops/perf/footprint_bench.sh profile-load-cache $(ARGS)
	ops/perf/footprint_bench.sh alloc cache

bench-footprint-update-baseline:
	@chmod +x ops/perf/footprint_bench.sh
	ops/perf/footprint_bench.sh update-baseline $(ARGS)

profile-summary:
	@chmod +x ops/perf/profile_summary.sh
	ops/perf/profile_summary.sh

full-check: check doc-check doc-rust

bump:
	@chmod +x scripts/bump-version.py
	python3 scripts/bump-version.py patch

bump-minor:
	@chmod +x scripts/bump-version.py
	python3 scripts/bump-version.py minor

bump-major:
	@chmod +x scripts/bump-version.py
	python3 scripts/bump-version.py major
