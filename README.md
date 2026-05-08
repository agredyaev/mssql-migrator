# Reporting Migrator

Go CLI for MSSQL reporting-layer migrations, validation, and reports.

## Build

```bash
go mod tidy
gofmt -w .
go test ./...
go vet ./...
go build -ldflags "-X main.version=0.1.0-dev -X main.commit=$(git rev-parse HEAD)" -o rmig ./cmd/rmig
```

## Commands

```bash
rmig version
rmig info --env prod
rmig plan --env prod
rmig migrate --env prod --plan-file reports/migration-plan.json
rmig validate --env prod
rmig baseline --env prod --up-to V010 --confirm
rmig repair-checksum --env prod --script R002__views.sql --confirm
```
