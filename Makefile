.PHONY: all build test vet lint fmt check db-up db-down db-init test-int test-int-v \
	bench-perf-diff-skip bench-perf-diff-create profile-perf-diff-skip-cpu profile-perf-diff-skip-mem \
	bench-perf-chunkkeys bench-perf-audit

STATICCHECK = $(shell go env GOPATH)/bin/staticcheck

# Performance harness: `internal/engine/benchmark_profiling_test.go`, `docs/profiling-benchmark-plan.md`,
# and audit backlog benches (`make bench-perf-audit`).
RMIG_BENCH_PKG := ./internal/engine
RMIG_TYPES_PKG := ./internal/types
RMIG_AUDIT_BENCH_PKGS := ./internal/bus ./internal/db ./internal/fs ./internal/apply
RMIG_PROFILE_DIR ?= /tmp/rmig-profiles

all: check

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint:
	$(STATICCHECK) ./...

fmt:
	test -z "$$(gofmt -l .)"

check: build test vet lint fmt

db-up:
	docker compose up -d
	@echo "Waiting for MSSQL to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		docker compose exec -T mssql /opt/mssql-tools18/bin/sqlcmd \
			-S localhost -U sa -P 'yourStrong(!)Password' -C \
			-Q "SELECT 1" >/dev/null 2>&1 && break; \
		sleep 3; \
	done
	@echo "Creating test database..."
	-docker compose exec -T mssql /opt/mssql-tools18/bin/sqlcmd \
		-S localhost -U sa -P 'yourStrong(!)Password' -C \
		-Q "IF DB_ID('rmig_test') IS NULL CREATE DATABASE rmig_test"

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

test-int-v: ARGS="-count=1"
test-int-v: test-int

# --- Performance (benchmem + optional pprof; not part of `check`) ---

bench-perf-diff-skip:
	go test $(RMIG_BENCH_PKG) -run '^$$' -bench '^BenchmarkDiffCompute_SkipHeavy_2000Objects$$' \
		-benchmem -count=5 -benchtime=400ms

bench-perf-diff-create:
	go test $(RMIG_BENCH_PKG) -run '^$$' -bench '^BenchmarkDiffCompute_Create_2000Objects$$' \
		-benchmem -count=5 -benchtime=400ms

bench-perf-chunkkeys:
	go test $(RMIG_TYPES_PKG) -run '^$$' -bench '^BenchmarkChunkKeys_10k_2100$$' \
		-benchmem -count=10 -benchtime=200ms

# Internal audit backlog: bus publish, inspector scope key / dual IN / cache, scanner git preload.
bench-perf-audit:
	go test $(RMIG_AUDIT_BENCH_PKGS) -run '^$$' \
		-bench '^BenchmarkBusPublish_|BenchmarkScopeKey_|BenchmarkBuildDualINQuery_|BenchmarkInspectorInspect_|BenchmarkScannerPreloadGitInfo_|BenchmarkLayoutRebuildPathIndexes_|BenchmarkCollectStatements_|BenchmarkExecuteTxBatch_' \
		-benchmem -count=5 -benchtime=150ms

profile-perf-diff-skip-cpu:
	@mkdir -p $(RMIG_PROFILE_DIR)
	go test $(RMIG_BENCH_PKG) -run '^$$' -bench '^BenchmarkDiffCompute_SkipHeavy_2000Objects$$' \
		-count=1 -benchtime=200ms -cpuprofile=$(RMIG_PROFILE_DIR)/cpu-skip-2000.prof
	@echo "Wrote $(RMIG_PROFILE_DIR)/cpu-skip-2000.prof — view: go tool pprof -http=:0 $(RMIG_PROFILE_DIR)/cpu-skip-2000.prof"

profile-perf-diff-skip-mem:
	@mkdir -p $(RMIG_PROFILE_DIR)
	go test $(RMIG_BENCH_PKG) -run '^$$' -bench '^BenchmarkDiffCompute_SkipHeavy_2000Objects$$' \
		-count=1 -benchtime=200ms -memprofile=$(RMIG_PROFILE_DIR)/mem-skip-2000.prof
	@echo "Wrote $(RMIG_PROFILE_DIR)/mem-skip-2000.prof — alloc_objects: go tool pprof -http=:0 -alloc_objects $(RMIG_PROFILE_DIR)/mem-skip-2000.prof"
