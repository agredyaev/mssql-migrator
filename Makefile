.PHONY: all build test vet lint fmt check db-up db-down db-init test-int test-int-v

STATICCHECK = $(shell go env GOPATH)/bin/staticcheck

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
	go test ./internal/migrator/ -run TestSQLServer -v $(ARGS)

test-int-v: ARGS="-count=1"
test-int-v: test-int
